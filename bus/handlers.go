package bus

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// handleList lists all messages in chronological order (newest first) and returns storage capacity headers.
func handleList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	channel, targetDir := getChannelDir(r)

	entries, err := os.ReadDir(targetDir)
	if err != nil {
		if os.IsNotExist(err) {
			w.Header().Set("X-Quota-Used", "0")
			w.Header().Set("X-Quota-Limit", strconv.FormatInt(maxDirSize, 10))
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]Message{})
			return
		}
		http.Error(w, "Failed to read storage directory", http.StatusInternalServerError)
		return
	}

	// Calculate total global size
	var totalSize int64
	filepath.Walk(filesDir, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			totalSize += info.Size()
		}
		return nil
	})

	messages := make([]Message, 0)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		msg, err := parseFilename(entry.Name(), targetDir)
		if err != nil {
			// Skip files that do not match the expected pattern
			continue
		}
		if channel != "" {
			msg.ID = channel + "/" + msg.ID
		}
		messages = append(messages, *msg)
	}

	// Sort chronologically (newest timestamp first)
	sort.Slice(messages, func(i, j int) bool {
		// Extract numeric timestamp from ID to sort securely
		return getTimestampFromID(messages[i].ID) > getTimestampFromID(messages[j].ID)
	})

	w.Header().Set("X-Quota-Used", strconv.FormatInt(totalSize, 10))
	w.Header().Set("X-Quota-Limit", strconv.FormatInt(maxDirSize, 10))
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(messages)
}

// handlePush creates a new text message or uploads a file, then cleans up space.
func handlePush(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	// Limit request size to prevent memory exhaustion
	r.Body = http.MaxBytesReader(w, r.Body, maxFileSize)
	if err := r.ParseMultipartForm(32 << 20); err != nil { // 32MB in-memory buffer
		http.Error(w, "Request too large or invalid multipart form", http.StatusBadRequest)
		return
	}

	device := r.FormValue("device")
	if device != "pc" && device != "mobile" && device != "ai" && device != "web" {
		device = "pc" // Default
	}

	now := time.Now().Unix()
	var createdID string
	var pushSize int64

	channel, targetDir := getChannelDir(r)
	if channel != "" {
		os.MkdirAll(targetDir, 0755)
	}

	// 1. Check for text push
	textVal := r.FormValue("text")
	if strings.TrimSpace(textVal) != "" {
		ts := uniqueTimestamp(targetDir, now, device, "text.txt")
		filename := fmt.Sprintf("%d_%s_text.txt", ts, device)
		filePath := filepath.Join(targetDir, filename)

		if err := os.WriteFile(filePath, []byte(textVal), 0644); err != nil {
			http.Error(w, "Failed to write text file", http.StatusInternalServerError)
			return
		}
		createdID = filename
		if channel != "" {
			createdID = channel + "/" + createdID
		}
		pushSize = int64(len(textVal))
	}

	// 2. Check for file push
	file, header, err := r.FormFile("file")
	if err == nil {
		defer file.Close()

		// Sanitize filename to avoid path injection
		origFilename := filepath.Base(header.Filename)
		// Clean up special characters, replace space with underscore
		reg := regexp.MustCompile(`[^a-zA-Z0-9.-_]`)
		origFilename = reg.ReplaceAllString(origFilename, "_")
		if origFilename == "" {
			origFilename = "unnamed_file"
		}

		ts := uniqueTimestamp(targetDir, now, device, origFilename)
		filename := fmt.Sprintf("%d_%s_%s", ts, device, origFilename)
		filePath := filepath.Join(targetDir, filename)

		out, err := os.Create(filePath)
		if err != nil {
			http.Error(w, "Failed to create destination file", http.StatusInternalServerError)
			return
		}
		defer out.Close()

		n, err := io.Copy(out, file)
		if err != nil {
			http.Error(w, "Failed to save file data", http.StatusInternalServerError)
			return
		}
		createdID = filename
		if channel != "" {
			createdID = channel + "/" + createdID
		}
		pushSize = n
	}

	if createdID == "" {
		http.Error(w, "Empty push request (no text or file)", http.StatusBadRequest)
		return
	}

	log.Printf("[%s] PUSH SUCCESS: device=%s, id=%s, size=%s", getClientIP(r), device, createdID, formatSize(pushSize))

	// Run rolling deletion to enforce max directory size
	go cleanupOldFiles()

	// Broadcast SSE event
	eventPayload := fmt.Sprintf("data: {\"event\":\"new_msg\",\"channel\":\"%s\",\"id\":\"%s\"}\n\n", channel, createdID)
	broadcaster.Broadcast(channel, eventPayload)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{
		"status": "success",
		"id":     createdID,
	})
}

// handleStream handles SSE long-polling connections.
func handleStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported!", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	channel := strings.TrimSpace(r.URL.Query().Get("channel"))

	client := &SSEClient{
		Channel: channel,
		Message: make(chan string, 10),
	}
	broadcaster.Register(client)
	defer broadcaster.Unregister(client)

	// Send initial connection success event
	fmt.Fprintf(w, "data: {\"event\":\"connected\"}\n\n")
	flusher.Flush()

	notify := r.Context().Done()
	for {
		select {
		case <-notify:
			return
		case msg := <-client.Message:
			fmt.Fprint(w, msg)
			flusher.Flush()
		}
	}
}

// handleAction processes pin, unpin, or delete on a specific card.
func handleAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	var payload ActionPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	safePath, err := getSafeFilePath(payload.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Ensure the file exists
	if _, err := os.Stat(safePath); os.IsNotExist(err) {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}

	switch payload.Action {
	case "pin":
		filename := filepath.Base(payload.ID)
		if !strings.HasPrefix(filename, "pinned_") {
			newID := "pinned_" + filename
			dirPath := filepath.Dir(payload.ID)
			if dirPath != "." && dirPath != "" && dirPath != string(filepath.Separator) {
				newID = filepath.ToSlash(filepath.Join(dirPath, newID))
			}
			newPath, err := getSafeFilePath(newID)
			if err != nil {
				http.Error(w, "Invalid target ID for pin", http.StatusBadRequest)
				return
			}
			if err := os.Rename(safePath, newPath); err != nil {
				http.Error(w, "Failed to pin file", http.StatusInternalServerError)
				return
			}
		}

	case "unpin":
		filename := filepath.Base(payload.ID)
		if newID, ok := strings.CutPrefix(filename, "pinned_"); ok {
			dirPath := filepath.Dir(payload.ID)
			if dirPath != "." && dirPath != "" && dirPath != string(filepath.Separator) {
				newID = filepath.ToSlash(filepath.Join(dirPath, newID))
			}
			newPath, err := getSafeFilePath(newID)
			if err != nil {
				http.Error(w, "Invalid target ID for unpin", http.StatusBadRequest)
				return
			}
			if err := os.Rename(safePath, newPath); err != nil {
				http.Error(w, "Failed to unpin file", http.StatusInternalServerError)
				return
			}
		}

	case "delete":
		if err := os.Remove(safePath); err != nil {
			http.Error(w, "Failed to delete file", http.StatusInternalServerError)
			return
		}

	default:
		http.Error(w, "Unknown action", http.StatusBadRequest)
		return
	}

	log.Printf("[%s] ACTION SUCCESS: action=%s, id=%s", getClientIP(r), payload.Action, payload.ID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}

// handleDownload serves physical files to the browser.
func handleDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	// The id is everything after "/api/download/"
	id := strings.TrimPrefix(r.URL.Path, "/api/download/")
	if id == "" {
		http.Error(w, "Missing file ID", http.StatusBadRequest)
		return
	}

	safePath, err := getSafeFilePath(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if _, err := os.Stat(safePath); os.IsNotExist(err) {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}

	log.Printf("[%s] DOWNLOAD: id=%s", getClientIP(r), id)

	// Parse out original filename for Content-Disposition header
	msg, err := parseFilename(filepath.Base(id), filepath.Dir(safePath))
	origFilename := filepath.Base(id)
	if err == nil {
		if msg.Type == "text" {
			origFilename = "text.txt"
		} else {
			origFilename = msg.Filename
		}
	}

	// Set content disposition to inline so it displays in browser if possible
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", origFilename))
	// Enable cache control for immutable files (our filenames contain epoch timestamp, so they are immutable)
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")

	http.ServeFile(w, r, safePath)
}

// LoginRequest is the JSON body for POST /api/login.
type LoginRequest struct {
	Password string `json:"password"`
	Key      string `json:"key"`
}

// LoginResponse is the JSON reply for POST /api/login.
type LoginResponse struct {
	Status string `json:"status"`
	Token  string `json:"token,omitempty"`
	Error  string `json:"error,omitempty"`
}

// handleLogin validates password and key with exponential backoff per IP.
func handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	ip := getClientIP(r)

	attemptsMu.Lock()
	attempt, exists := ipAttempts[ip]
	attemptsMu.Unlock()

	now := time.Now()
	if exists && attempt.FailCount > 0 {
		// delay = 2^(FailCount-1) seconds
		delaySec := 1 << (attempt.FailCount - 1)
		delaySec = min(delaySec, 60)

		elapsed := now.Sub(attempt.LastTime)
		remaining := time.Duration(delaySec)*time.Second - elapsed
		if remaining > 0 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			json.NewEncoder(w).Encode(LoginResponse{
				Status: "error",
				Error:  fmt.Sprintf("尝试次数过多，请等待 %d 秒后重试", int(remaining.Seconds())+1),
			})
			return
		}
	}

	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(LoginResponse{Status: "error", Error: "Invalid request payload"})
		return
	}

	if strings.TrimSpace(req.Password) == "" || strings.TrimSpace(req.Key) == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(LoginResponse{Status: "error", Error: "Password and URL key cannot be empty"})
		return
	}

	if req.Password == config.Password && req.Key == config.LoginKey {
		// Success: reset attempts
		attemptsMu.Lock()
		delete(ipAttempts, ip)
		attemptsMu.Unlock()

		log.Printf("[%s] LOGIN SUCCESS: with key=%s", ip, req.Key)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(LoginResponse{Status: "success", Token: config.Token})
		return
	}

	// Failure: record attempt
	attemptsMu.Lock()
	attempt.FailCount++
	attempt.LastTime = now
	ipAttempts[ip] = attempt
	attemptsMu.Unlock()

	log.Printf("[%s] Failed login attempt with key: %s (Fail count: %d)", ip, req.Key, attempt.FailCount)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	json.NewEncoder(w).Encode(LoginResponse{Status: "error", Error: "密码或 URL 参数错误"})
}
