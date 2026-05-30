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
}

// ActionPayload is the JSON structure for pin/unpin/delete operations
type ActionPayload struct {
	ID     string `json:"id"`
	Action string `json:"action"` // "pin", "unpin", "delete"
}

// Config represents the application configuration format
type Config struct {
	Token    string `json:"token"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

var (
	globalConfig Config
	maxDirSize   int64 = 2 * 1024 * 1024 * 1024 // 2 GB
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
	mux.HandleFunc("/api/login", handleLogin)
	mux.HandleFunc("/download/", handleDownload)
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
	defaultEmail := "284420441@qq.com"
	defaultPassword := "66666666"

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		token := generateRandomToken()
		cfg := Config{
			Token:    token,
			Email:    defaultEmail,
			Password: defaultPassword,
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
			Email:    defaultEmail,
			Password: defaultPassword,
		}
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil || cfg.Token == "" {
		log.Printf("Warning: invalid config.json. Using defaults.")
		return Config{
			Token:    "temporary_token",
			Email:    defaultEmail,
			Password: defaultPassword,
		}
	}

	if cfg.Email == "" {
		cfg.Email = defaultEmail
	}
	if cfg.Password == "" {
		cfg.Password = defaultPassword
	}

	// Save back to config.json if there were missing fields
	if cfg.Email == defaultEmail || cfg.Password == defaultPassword {
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

// getSafeFilePath validates and returns a clean, safe path to a file inside files/
func getSafeFilePath(id string) (string, error) {
	// Prevent path traversal
	if strings.Contains(id, "/") || strings.Contains(id, "\\") || id == ".." || id == "." {
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
func parseFilename(filename string) (*Message, error) {
	// Standard: [pinned_]?[timestamp]_[original_name]
	msg := &Message{
		ID: filename,
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
	origName := workingName[idx+1:]

	ts, err := strconv.ParseInt(tsStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("failed to parse timestamp")
	}

	msg.Time = time.Unix(ts, 0).Format("2006-01-02 15:04:05")

	if origName == "text.txt" {
		msg.Type = "text"
		// Read content
		filePath := filepath.Join(filesDir, filename)
		contentBytes, err := os.ReadFile(filePath)
		if err != nil {
			msg.Content = "[Error: failed to read text message]"
		} else {
			msg.Content = string(contentBytes)
		}
	} else {
		msg.Type = "file"
		msg.Filename = origName
		// Get size
		filePath := filepath.Join(filesDir, filename)
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

// handleList lists all messages in chronological order (newest first)
func handleList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	entries, err := os.ReadDir(filesDir)
	if err != nil {
		http.Error(w, "Failed to read storage directory", http.StatusInternalServerError)
		return
	}

	messages := make([]Message, 0)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		msg, err := parseFilename(entry.Name())
		if err != nil {
			// Skip files that do not match the expected pattern
			continue
		}
		messages = append(messages, *msg)
	}

	// Sort chronologically (newest timestamp first)
	sort.Slice(messages, func(i, j int) bool {
		// Extract numeric timestamp from ID to sort securely
		return getTimestampFromID(messages[i].ID) > getTimestampFromID(messages[j].ID)
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(messages)
}

func getTimestampFromID(id string) int64 {
	name := id
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

	now := time.Now().Unix()
	var createdID string

	// 1. Check for text push
	textVal := r.FormValue("text")
	if strings.TrimSpace(textVal) != "" {
		filename := fmt.Sprintf("%d_text.txt", now)
		filePath := filepath.Join(filesDir, filename)

		if err := os.WriteFile(filePath, []byte(textVal), 0644); err != nil {
			http.Error(w, "Failed to write text file", http.StatusInternalServerError)
			return
		}
		createdID = filename
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

		filename := fmt.Sprintf("%d_%s", now, origFilename)
		filePath := filepath.Join(filesDir, filename)

		out, err := os.Create(filePath)
		if err != nil {
			http.Error(w, "Failed to create destination file", http.StatusInternalServerError)
			return
		}
		defer out.Close()

		if _, err := io.Copy(out, file); err != nil {
			http.Error(w, "Failed to save file data", http.StatusInternalServerError)
			return
		}
		createdID = filename
	}

	if createdID == "" {
		http.Error(w, "Empty push request (no text or file)", http.StatusBadRequest)
		return
	}

	// Run rolling deletion to enforce max directory size
	go cleanupOldFiles()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{
		"status": "success",
		"id":     createdID,
	})
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
		if !strings.HasPrefix(payload.ID, "pinned_") {
			newID := "pinned_" + payload.ID
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
		if strings.HasPrefix(payload.ID, "pinned_") {
			newID := strings.TrimPrefix(payload.ID, "pinned_")
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

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}

// handleDownload serves physical files to the browser
func handleDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	// The id is everything after "/download/"
	id := strings.TrimPrefix(r.URL.Path, "/download/")
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

	// Parse out original filename for Content-Disposition header
	msg, err := parseFilename(id)
	origFilename := id
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
	entries, err := os.ReadDir(filesDir)
	if err != nil {
		log.Printf("Cleanup warning: failed to read files directory: %v", err)
		return
	}

	type fileInfo struct {
		name string
		size int64
		ts   int64
	}

	var totalSize int64
	unpinnedFiles := make([]fileInfo, 0)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		totalSize += info.Size()

		// Filter unpinned files
		if !strings.HasPrefix(entry.Name(), "pinned_") {
			ts := getTimestampFromID(entry.Name())
			unpinnedFiles = append(unpinnedFiles, fileInfo{
				name: entry.Name(),
				size: info.Size(),
				ts:   ts,
			})
		}
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

		path := filepath.Join(filesDir, f.name)
		if err := os.Remove(path); err == nil {
			totalSize -= f.size
			log.Printf("Cleaned up oldest unpinned file: %s (size: %s)", f.name, formatSize(f.size))
		} else {
			log.Printf("Cleanup warning: failed to delete file %s: %v", f.name, err)
		}
	}
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginResponse struct {
	Status string `json:"status"`
	Token  string `json:"token,omitempty"`
	Error  string `json:"error,omitempty"`
}

// handleLogin validates email and password, and returns the secure token if valid
func handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(LoginResponse{Status: "error", Error: "Invalid request payload"})
		return
	}

	if strings.TrimSpace(req.Email) == "" || strings.TrimSpace(req.Password) == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(LoginResponse{Status: "error", Error: "Email and password cannot be empty"})
		return
	}

	if req.Email == globalConfig.Email && req.Password == globalConfig.Password {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(LoginResponse{Status: "success", Token: globalConfig.Token})
		return
	}

	log.Printf("[%s] Failed login attempt for email: %s", r.RemoteAddr, req.Email)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	json.NewEncoder(w).Encode(LoginResponse{Status: "error", Error: "Invalid email or password"})
}
