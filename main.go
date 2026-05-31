package main

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
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	filesDir    = "./files"
	staticDir   = "./static"
	port        = ":6666"
	maxFileSize = 500 * 1024 * 1024 // 500 MB maximum per upload
)

// Message represents the JSON structure of a file/text bubble in the timeline
type Message struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Filename string `json:"filename,omitempty"`
	Content  string `json:"content,omitempty"`
	Size     string `json:"size,omitempty"`
	Time     string `json:"time"`
	Pinned   bool   `json:"pinned"`
	Device   string `json:"device"`
}

// ActionPayload is the JSON structure for pin/unpin/delete operations
type ActionPayload struct {
	ID     string `json:"id"`
	Action string `json:"action"` // "pin", "unpin", "delete"
}

// Config represents the application configuration format
type Config struct {
	Token    string `json:"token"`
	Password string `json:"password"`
	LoginKey string `json:"login_key"`
}

type IPAttempt struct {
	FailCount int
	LastTime  time.Time
}

type SSEClient struct {
	Channel string
	Message chan string
}

type Broadcaster struct {
	clients sync.Map
}

func (b *Broadcaster) Register(client *SSEClient) {
	b.clients.Store(client, true)
}

func (b *Broadcaster) Unregister(client *SSEClient) {
	b.clients.Delete(client)
	close(client.Message)
}

func (b *Broadcaster) Broadcast(channel string, eventPayload string) {
	b.clients.Range(func(key, value interface{}) bool {
		client := key.(*SSEClient)
		if client.Channel == "" || client.Channel == channel {
			select {
			case client.Message <- eventPayload:
			default:
				// Drop if channel is full to prevent blocking
			}
		}
		return true
	})
}

var (
	globalConfig Config
	maxDirSize   int64 = 2 * 1024 * 1024 * 1024 // 2 GB
	attemptsMu   sync.Mutex
	ipAttempts   = make(map[string]IPAttempt)
	globalBroadcaster = &Broadcaster{}
)

func main() {
	// Ensure directories exist
	if err := os.MkdirAll(filesDir, 0755); err != nil {
		log.Fatalf("Failed to create files directory: %v", err)
	}
	if err := os.MkdirAll(staticDir, 0755); err != nil {
		log.Fatalf("Failed to create static directory: %v", err)
	}

	// Load or generate static security config
	globalConfig = loadOrGenerateConfig()

	// Setup clean environment variable limit override if needed (e.g. for testing)
	if envLimit := os.Getenv("ICS_MAX_DIR_SIZE_BYTES"); envLimit != "" {
		if val, err := strconv.ParseInt(envLimit, 10, 64); err == nil {
			maxDirSize = val
			log.Printf("Custom Max Directory Size configured: %d bytes", maxDirSize)
		}
	}

	mux := http.NewServeMux()

	// Register routes
	mux.HandleFunc("/api/list", handleList)
	mux.HandleFunc("/api/push", handlePush)
	mux.HandleFunc("/api/action", handleAction)
	mux.HandleFunc("/api/stream", handleStream)
	mux.HandleFunc("/api/login", handleLogin)
	mux.HandleFunc("/api/download/", handleDownload)
	mux.HandleFunc("/", serveStatic)

	// Apply Token authentication middleware
	handlerWithMiddleware := tokenAuthMiddleware(mux, globalConfig.Token)

	// Listen only on 127.0.0.1 for maximum loopback security (Cloudflare Tunnel forwards locally)
	bindAddr := "127.0.0.1" + port
	log.Printf("Starting ICS-Core server locally on %s...", bindAddr)
	if err := http.ListenAndServe(bindAddr, handlerWithMiddleware); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}

// generateRandomToken creates a cryptographically secure 32-character token
func generateRandomToken() string {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "default_secure_token_9981"
	}
	return hex.EncodeToString(bytes)
}

// loadOrGenerateConfig loads config from config.json or generates a new one
func loadOrGenerateConfig() Config {
	configPath := "./config.json"
	defaultPassword := "66666666"
	defaultLoginKey := "vip"

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		token := generateRandomToken()
		cfg := Config{
			Token:    token,
			Password: defaultPassword,
			LoginKey: defaultLoginKey,
		}
		data, _ := json.MarshalIndent(cfg, "", "  ")
		_ = os.WriteFile(configPath, data, 0600)
		log.Printf("Generated new secure config in config.json with token: %s", token)
		return cfg
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		log.Printf("Warning: failed to read config.json: %v. Using defaults.", err)
		return Config{
			Token:    "temporary_token",
			Password: defaultPassword,
			LoginKey: defaultLoginKey,
		}
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil || cfg.Token == "" {
		log.Printf("Warning: invalid config.json. Using defaults.")
		return Config{
			Token:    "temporary_token",
			Password: defaultPassword,
			LoginKey: defaultLoginKey,
		}
	}

	if cfg.Password == "" {
		cfg.Password = defaultPassword
	}
	if cfg.LoginKey == "" {
		cfg.LoginKey = defaultLoginKey
	}

	// Save back to config.json if there were missing fields
	if cfg.Password == defaultPassword || cfg.LoginKey == defaultLoginKey {
		data, _ = json.MarshalIndent(cfg, "", "  ")
		_ = os.WriteFile(configPath, data, 0600)
	}

	return cfg
}

// tokenAuthMiddleware enforces secret Token checks for all private API routes
func tokenAuthMiddleware(next http.Handler, secretToken string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Exempt static frontend files and login endpoint from token validation
		path := r.URL.Path
		if path == "/" || path == "/index.html" || strings.HasPrefix(path, "/static/") || path == "/api/login" {
			next.ServeHTTP(w, r)
			return
		}

		// Retrieve token from header or query param
		token := r.Header.Get("X-Auth-Token")
		if token == "" {
			token = r.URL.Query().Get("token")
		}

		if token != secretToken {
			log.Printf("[%s] Blocked Unauthorized request: %s %s (Token mismatch)", r.RemoteAddr, r.Method, r.URL.Path)
			http.Error(w, "Unauthorized: Invalid or missing token", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// serveStatic serves index.html or other files in static directory
func serveStatic(w http.ResponseWriter, r *http.Request) {
	// Default to index.html
	path := r.URL.Path
	if path == "/" || path == "" {
		path = "/index.html"
	}

	safePath := filepath.Join(staticDir, filepath.Base(path))
	if _, err := os.Stat(safePath); os.IsNotExist(err) {
		// If not found, write a minimal blank page to let user know it runs
		if path == "/index.html" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			fmt.Fprint(w, "<h1>ICS Running. Place index.html in ./static/</h1>")
			return
		}
		http.NotFound(w, r)
		return
	}

	http.ServeFile(w, r, safePath)
}

// getChannelDir extracts the channel query param and returns channel name and safe target directory
func getChannelDir(r *http.Request) (string, string) {
	channel := strings.TrimSpace(r.URL.Query().Get("channel"))
	if channel != "" {
		channel = strings.ReplaceAll(channel, "/", "")
		channel = strings.ReplaceAll(channel, "\\", "")
		channel = strings.ReplaceAll(channel, "..", "")
		return channel, filepath.Join(filesDir, channel)
	}
	return "", filesDir
}

// getSafeFilePath validates and returns a clean, safe path to a file inside files/
func getSafeFilePath(id string) (string, error) {
	// Prevent path traversal
	if strings.Contains(id, "..") {
		return "", fmt.Errorf("invalid file ID")
	}

	safePath := filepath.Clean(filepath.Join(filesDir, id))
	absFilesDir, err := filepath.Abs(filesDir)
	if err != nil {
		return "", err
	}
	absSafePath, err := filepath.Abs(safePath)
	if err != nil {
		return "", err
	}

	if !strings.HasPrefix(absSafePath, absFilesDir) {
		return "", fmt.Errorf("path traversal detected")
	}

	return safePath, nil
}

// parseFilename parses metadata out of the actual filesystem filename
func parseFilename(filename string, targetDir string) (*Message, error) {
	// Standard: [pinned_]?[timestamp]_[device]_[original_name] or legacy: [pinned_]?[timestamp]_[original_name]
	msg := &Message{
		ID:     filename,
		Device: "pc", // default
	}

	workingName := filename
	if strings.HasPrefix(workingName, "pinned_") {
		msg.Pinned = true
		workingName = strings.TrimPrefix(workingName, "pinned_")
	}

	// Split by first underscore to get timestamp
	idx := strings.Index(workingName, "_")
	if idx == -1 {
		return nil, fmt.Errorf("invalid filename pattern")
	}

	tsStr := workingName[:idx]
	rest := workingName[idx+1:]

	ts, err := strconv.ParseInt(tsStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("failed to parse timestamp")
	}

	msg.Time = time.Unix(ts, 0).Format("2006-01-02 15:04:05")

	// Check if there is a device tag
	origName := rest
	idx2 := strings.Index(rest, "_")
	if idx2 != -1 {
		possibleDevice := rest[:idx2]
		if possibleDevice == "pc" || possibleDevice == "mobile" || possibleDevice == "ai" || possibleDevice == "web" {
			msg.Device = possibleDevice
			origName = rest[idx2+1:]
		}
	}

	if origName == "text.txt" {
		msg.Type = "text"
		filePath := filepath.Join(targetDir, filename)
		info, err := os.Stat(filePath)
		if err != nil {
			msg.Content = "[Error: failed to read text message info]"
		} else {
			fileSize := info.Size()
			const limit = 10 * 1024 // 10 KB
			if fileSize <= limit {
				contentBytes, err := os.ReadFile(filePath)
				if err != nil {
					msg.Content = "[Error: failed to read text message]"
				} else {
					msg.Content = string(contentBytes)
				}
			} else {
				f, err := os.Open(filePath)
				if err != nil {
					msg.Content = "[Error: failed to open text message]"
				} else {
					defer f.Close()
					buf := make([]byte, limit)
					n, err := f.Read(buf)
					if err != nil && err != io.EOF {
						msg.Content = "[Error: failed to read partial text message]"
					} else {
						msg.Content = string(buf[:n]) + "\n... [内容过长已截断，请下载完整文件]"
					}
				}
			}
		}
	} else {
		msg.Type = "file"
		msg.Filename = origName
		// Get size
		filePath := filepath.Join(targetDir, filename)
		info, err := os.Stat(filePath)
		if err == nil {
			msg.Size = formatSize(info.Size())
		} else {
			msg.Size = "Unknown Size"
		}
	}

	return msg, nil
}

func formatSize(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %cB", float64(size)/float64(div), "KMGTPE"[exp])
}

// handleList lists all messages in chronological order (newest first) and returns storage capacity headers
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

func getTimestampFromID(id string) int64 {
	name := filepath.Base(id)
	if strings.HasPrefix(name, "pinned_") {
		name = strings.TrimPrefix(name, "pinned_")
	}
	idx := strings.Index(name, "_")
	if idx == -1 {
		return 0
	}
	ts, _ := strconv.ParseInt(name[:idx], 10, 64)
	return ts
}

// handlePush creates a new text message or uploads a file, then cleans up space
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
		filename := fmt.Sprintf("%d_%s_text.txt", now, device)
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

		filename := fmt.Sprintf("%d_%s_%s", now, device, origFilename)
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
	globalBroadcaster.Broadcast(channel, eventPayload)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{
		"status": "success",
		"id":     createdID,
	})
}

// handleStream handles SSE long-polling connections
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
	globalBroadcaster.Register(client)
	defer globalBroadcaster.Unregister(client)

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

// handleAction processes pin, unpin, or delete on a specific card
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
		if strings.HasPrefix(filename, "pinned_") {
			newID := strings.TrimPrefix(filename, "pinned_")
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

// handleDownload serves physical files to the browser
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

// cleanupOldFiles traverses filesDir and removes oldest unpinned files if total exceeds limit
func cleanupOldFiles() {
	type fileInfo struct {
		path string
		name string
		size int64
		ts   int64
	}

	var totalSize int64
	unpinnedFiles := make([]fileInfo, 0)

	err := filepath.Walk(filesDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		totalSize += info.Size()
		name := filepath.Base(path)
		if !strings.HasPrefix(name, "pinned_") {
			ts := getTimestampFromID(name)
			unpinnedFiles = append(unpinnedFiles, fileInfo{
				path: path,
				name: name,
				size: info.Size(),
				ts:   ts,
			})
		}
		return nil
	})

	if err != nil {
		log.Printf("Cleanup warning: failed to walk files directory: %v", err)
	}

	log.Printf("Current Storage Size: %s / Max quota: %s", formatSize(totalSize), formatSize(maxDirSize))

	if totalSize <= maxDirSize {
		return
	}

	// Sort unpinned files by timestamp ascending (oldest first)
	sort.Slice(unpinnedFiles, func(i, j int) bool {
		return unpinnedFiles[i].ts < unpinnedFiles[j].ts
	})

	for _, f := range unpinnedFiles {
		if totalSize <= maxDirSize {
			break
		}

		if err := os.Remove(f.path); err == nil {
			totalSize -= f.size
			log.Printf("[SYSTEM] CLEANUP: removed oldest unpinned file %s (size: %s)", f.name, formatSize(f.size))
		} else {
			log.Printf("[SYSTEM] CLEANUP WARNING: failed to delete file %s: %v", f.name, err)
		}
	}
}

type LoginRequest struct {
	Password string `json:"password"`
	Key      string `json:"key"`
}

type LoginResponse struct {
	Status string `json:"status"`
	Token  string `json:"token,omitempty"`
	Error  string `json:"error,omitempty"`
}

// getClientIP extracts client IP address taking into account Cloudflare headers
func getClientIP(r *http.Request) string {
	if cfIP := r.Header.Get("CF-Connecting-IP"); cfIP != "" {
		return cfIP
	}
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}
	parts := strings.Split(r.RemoteAddr, ":")
	if len(parts) > 0 {
		return parts[0]
	}
	return r.RemoteAddr
}

// handleLogin validates password and key with exponential backoff per IP
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
		if delaySec > 60 {
			delaySec = 60
		}

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

	if req.Password == globalConfig.Password && req.Key == globalConfig.LoginKey {
		// Success: reset attempts
		attemptsMu.Lock()
		delete(ipAttempts, ip)
		attemptsMu.Unlock()

		log.Printf("[%s] LOGIN SUCCESS: with key=%s", ip, req.Key)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(LoginResponse{Status: "success", Token: globalConfig.Token})
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
