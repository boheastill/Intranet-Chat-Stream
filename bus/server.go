// Package bus implements the ICS Message Bus — a dumb pipe that stores and
// serves cross-device messages over REST + SSE. It knows nothing about who
// reads or writes, or what messages mean.
package bus

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

const (
	staticDir   = "./static"
	maxFileSize = 500 * 1024 * 1024 // 500 MB maximum per upload
)

// filesDir is a variable (not const) so tests can point it at a temp dir.
var filesDir = "./files"

// config holds the active server configuration, injected via Run.
var config Config

// Run starts the Message Bus HTTP server and blocks until it exits.
func Run(cfg Config) error {
	config = cfg

	// Ensure directories exist
	if err := os.MkdirAll(filesDir, 0755); err != nil {
		return fmt.Errorf("failed to create files directory: %w", err)
	}
	if err := os.MkdirAll(staticDir, 0755); err != nil {
		return fmt.Errorf("failed to create static directory: %w", err)
	}

	// Allow overriding the storage quota (e.g. for testing)
	if envLimit := os.Getenv("ICS_MAX_DIR_SIZE_BYTES"); envLimit != "" {
		if val, err := strconv.ParseInt(envLimit, 10, 64); err == nil {
			maxDirSize = val
			log.Printf("Custom Max Directory Size configured: %d bytes", maxDirSize)
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/list", handleList)
	mux.HandleFunc("/api/push", handlePush)
	// Chunked upload path: lets large files cross proxies that cap a single
	// request body (e.g. Cloudflare's 100 MB per-request limit on free plans).
	mux.HandleFunc("/api/push/chunk/init", handleChunkInit)
	mux.HandleFunc("/api/push/chunk/status", handleChunkStatus)
	mux.HandleFunc("/api/push/chunk/data/", handleChunkData)
	mux.HandleFunc("/api/push/chunk/complete", handleChunkComplete)
	mux.HandleFunc("/api/action", handleAction)
	mux.HandleFunc("/api/stream", handleStream)
	mux.HandleFunc("/api/login", handleLogin)
	mux.HandleFunc("/api/download/", handleDownload)
	mux.HandleFunc("/", serveStatic)

	handler := tokenAuthMiddleware(mux, config.Token)

	// Janitor: expire abandoned chunked uploads and their temp files
	go func() {
		for range time.Tick(time.Hour) {
			sweepStaleChunks()
		}
	}()

	// Listen only on 127.0.0.1 (Cloudflare Tunnel forwards locally)
	bindAddr := fmt.Sprintf("127.0.0.1:%d", config.Port)
	log.Printf("Starting ICS Message Bus locally on %s...", bindAddr)
	log.Printf("Web UI (logged in): http://%s/?token=%s", bindAddr, config.Token)
	return http.ListenAndServe(bindAddr, handler)
}

// serveStatic serves index.html or other files in the static directory.
func serveStatic(w http.ResponseWriter, r *http.Request) {
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

	// index.html is the app shell — always revalidate so deploys take effect immediately
	if filepath.Base(safePath) == "index.html" {
		w.Header().Set("Cache-Control", "no-cache")
	}
	http.ServeFile(w, r, safePath)
}
