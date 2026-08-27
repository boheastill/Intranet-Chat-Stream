package bus

// Chunked upload support: files larger than a proxy's per-request cap (e.g.
// Cloudflare free plans reject bodies over 100 MB with 413 at the edge) are
// sent as a sequence of small data requests appended to one temp file, then
// finalized into the normal file store. State is in-memory + temp dir; an
// abandoned upload is swept by the hourly janitor.

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	chunkRequestCap = 16 << 20        // hard cap on any single data request body
	chunkIdleExpiry = 24 * time.Hour  // idle sessions/temps are swept
)

var chunkIDRe = regexp.MustCompile(`^[0-9a-f]{32}$`)

type chunkSession struct {
	Filename   string
	Size       int64
	Written    int64
	Device     string
	Channel    string
	TargetDir  string
	TempPath   string
	LastActive time.Time
}

var (
	uploadsMu sync.Mutex
	uploads   = map[string]*chunkSession{}
)

func chunksRoot() string { return filepath.Join(filesDir, ".chunks") }

// sanitizeFilename strips path components and special characters, matching
// how direct /api/push handles multipart filenames.
func sanitizeFilename(name string) string {
	base := filepath.Base(name)
	reg := regexp.MustCompile(`[^a-zA-Z0-9.-_]`)
	base = reg.ReplaceAllString(base, "_")
	if base == "" {
		return "unnamed_file"
	}
	return base
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(payload)
}

// handleChunkInit creates an upload slot and its temp file.
// POST /api/push/chunk/init  {"filename","size","device"?,"channel"?}
func handleChunkInit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Filename string `json:"filename"`
		Size     int64  `json:"size"`
		Device   string `json:"device"`
		Channel  string `json:"channel"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&req); err != nil || req.Size <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid init payload"})
		return
	}
	if req.Size > maxFileSize {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": fmt.Sprintf("size exceeds %d byte limit", maxFileSize)})
		return
	}

	device := req.Device
	if device != "pc" && device != "mobile" && device != "ai" && device != "web" {
		device = "pc"
	}

	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "id generation failed"})
		return
	}
	id := hex.EncodeToString(buf)

	channel, targetDir := getChannelDir(r)
	if channel != "" {
		os.MkdirAll(targetDir, 0755)
	}

	dir := filepath.Join(chunksRoot(), id)
	if err := os.MkdirAll(dir, 0755); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "temp dir creation failed"})
		return
	}

	sess := &chunkSession{
		Filename:  filepath.Base(req.Filename),
		Size:      req.Size,
		Device:    device,
		Channel:   channel,
		TargetDir: targetDir,
		TempPath:  filepath.Join(dir, "data.bin"),
	}
	now := time.Now()
	sess.LastActive = now

	uploadsMu.Lock()
	uploads[id] = sess
	uploadsMu.Unlock()

	log.Printf("[%s] CHUNK INIT: id=%s, name=%s, size=%s", getClientIP(r), id, sess.Filename, formatSize(req.Size))
	writeJSON(w, http.StatusCreated, map[string]string{"upload_id": id})
}

// handleChunkStatus reports how many bytes the server holds for an upload,
// so clients can resume after a dropped connection.
// GET /api/push/chunk/status?id=<hex>
func handleChunkStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	id := r.URL.Query().Get("id")
	if !chunkIDRe.MatchString(id) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad upload id"})
		return
	}

	uploadsMu.Lock()
	sess, ok := uploads[id]
	if ok {
		sess.LastActive = time.Now()
		written := sess.Written
		total := sess.Size
		uploadsMu.Unlock()
		writeJSON(w, http.StatusOK, map[string]int64{"written": written, "size": total})
		return
	}
	uploadsMu.Unlock()
	writeJSON(w, http.StatusNotFound, map[string]string{"error": "unknown upload id"})
}

// handleChunkData appends raw bytes sequentially to an upload's temp file.
// PUT /api/push/chunk/data/<id>?offset=N   body: application/octet-stream
func handleChunkData(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut && r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/push/chunk/data/")
	if !chunkIDRe.MatchString(id) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad upload id"})
		return
	}

	var offset int64
	fmt.Sscanf(r.URL.Query().Get("offset"), "%d", &offset)

	uploadsMu.Lock()
	sess, ok := uploads[id]
	if !ok {
		uploadsMu.Unlock()
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "unknown or expired upload"})
		return
	}
	sess.LastActive = time.Now()
	expected := sess.Written
	remaining := sess.Size - expected
	uploadsMu.Unlock()

	if offset != expected {
		writeJSON(w, http.StatusConflict, map[string]any{"error": "offset mismatch", "expected": expected})
		return
	}
	if remaining <= 0 {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "declared size already reached"})
		return
	}

	capBytes := remaining
	if capBytes > chunkRequestCap {
		capBytes = chunkRequestCap
	}
	r.Body = http.MaxBytesReader(w, r.Body, capBytes)

	f, err := os.OpenFile(sess.TempPath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "temp file open failed"})
		return
	}
	defer f.Close()

	n, copyErr := io.Copy(f, r.Body)

	// Disk state is the source of truth for resumability. A mid-request drop
	// leaves a legal partial append — keep it. Bytes beyond the declared total
	// are rolled back so the session can never overshoot its own Size.
	size := int64(n)
	if st, statErr := f.Stat(); statErr == nil {
		size = st.Size()
	}
	if size > sess.Size {
		f.Truncate(sess.Size)
		size = sess.Size
	}
	uploadsMu.Lock()
	sess.Written = size
	totalNow := size
	uploadsMu.Unlock()

	if copyErr != nil || n > remaining {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error":   "chunk rejected",
			"written": fmt.Sprint(totalNow),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]int64{"written": totalNow})
}

// handleChunkComplete validates the total and moves the temp file into the
// live store exactly like a direct /api/push would have.
// POST /api/push/chunk/complete  {"id"}
func handleChunkComplete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 16<<10)).Decode(&req); err != nil || !chunkIDRe.MatchString(req.ID) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad complete payload"})
		return
	}

	uploadsMu.Lock()
	sess, ok := uploads[req.ID]
	delete(uploads, req.ID)
	uploadsMu.Unlock()
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "unknown or expired upload"})
		return
	}

	defer os.RemoveAll(filepath.Dir(sess.TempPath))

	if sess.Written != sess.Size {
		writeJSON(w, http.StatusConflict, map[string]any{
			"error": "incomplete upload", "written": sess.Written, "expected": sess.Size})
		return
	}

	origFilename := sanitizeFilename(sess.Filename)
	ts := uniqueTimestamp(sess.TargetDir, time.Now().Unix(), sess.Device, origFilename)
	filename := fmt.Sprintf("%d_%s_%s", ts, sess.Device, origFilename)
	finalPath := filepath.Join(sess.TargetDir, filename)

	if err := os.Rename(sess.TempPath, finalPath); err != nil {
		log.Printf("CHUNK COMPLETE failed to promote id=%s: %v", req.ID, err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to finalize file"})
		return
	}

	id := filename
	if sess.Channel != "" {
		id = sess.Channel + "/" + id
	}
	log.Printf("[%s] CHUNK COMPLETE: device=%s, id=%s, size=%s", getClientIP(r), sess.Device, id, formatSize(sess.Size))

	go cleanupOldFiles()

	eventPayload := fmt.Sprintf("data: {\"event\":\"new_msg\",\"channel\":\"%s\",\"id\":\"%s\"}\n\n", sess.Channel, id)
	broadcaster.Broadcast(sess.Channel, eventPayload)

	writeJSON(w, http.StatusCreated, map[string]string{"status": "success", "id": id})
}

// sweepStaleChunks drops idle sessions and orphaned temp dirs (server restarts
// leave dirs behind without in-memory sessions).
func sweepStaleChunks() {
	cutoff := time.Now().Add(-chunkIdleExpiry)

	uploadsMu.Lock()
	for id, sess := range uploads {
		if sess.LastActive.Before(cutoff) {
			os.RemoveAll(filepath.Dir(sess.TempPath))
			delete(uploads, id)
			log.Printf("CHUNK SWEEP: expired session id=%s", id)
		}
	}
	uploadsMu.Unlock()

	entries, err := os.ReadDir(chunksRoot())
	if err != nil {
		return
	}
	for _, e := range entries {
		info, err := e.Info()
		if err != nil || info.ModTime().After(cutoff) {
			continue
		}
		uploadsMu.Lock()
		_, live := uploads[e.Name()]
		uploadsMu.Unlock()
		if !live {
			os.RemoveAll(filepath.Join(chunksRoot(), e.Name()))
			log.Printf("CHUNK SWEEP: removed orphan dir %s", e.Name())
		}
	}
}
