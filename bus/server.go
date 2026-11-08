// Package bus implements the ICS Message Bus — a dumb pipe that stores and
// serves cross-device messages over REST + SSE. It knows nothing about who
// reads or writes, or what messages mean.
package bus

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"syscall"
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
	mux.HandleFunc("/api/channels", handleChannels)
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

	// Order: observability outermost so it sees the final status of every
	// request, including ones rejected by the middlewares below it.
	handler := observe(securityHeaders(tokenAuthMiddleware(mux, config.Token)))

	// Janitor: expire abandoned chunked uploads and their temp files
	go func() {
		for range time.Tick(time.Hour) {
			sweepStaleChunks()
		}
	}()

	// Listen only on 127.0.0.1 (Cloudflare Tunnel forwards locally)
	bindAddr := fmt.Sprintf("127.0.0.1:%d", config.Port)

	// One startup line with everything an operator needs after a reboot, so the
	// journal alone answers "what is running, with which settings".
	log.Printf("STARTUP ics %s | listen=%s | storage=%s quota=%s | channels=enabled",
		buildVersion, bindAddr, filesDir, formatSize(maxDirSize))
	log.Printf("STARTUP files in store: %d | login key length: %d | password length: %d",
		countFiles(), len(config.LoginKey), len(config.Password))

	// Say why the process stopped: without this, a reboot, an OOM kill and a
	// deploy all look identical in the journal.
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigs
		log.Printf("SHUTDOWN received %s | uptime=%s", sig, time.Since(startedAt).Round(time.Second))
		os.Exit(0)
	}()
	startedAt = time.Now()

	return http.ListenAndServe(bindAddr, handler)
}

// buildVersion is reported at startup so the journal records which build is
// actually running (deploys otherwise look identical).
var buildVersion = "dev"

var startedAt = time.Now()

// countFiles is a cheap startup census of the message store.
func countFiles() int {
	n := 0
	filepath.Walk(filesDir, func(_ string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			n++
		}
		return nil
	})
	return n
}

// statusRecorder captures what was actually sent so observe() can log failures
// without every handler having to remember. Flush/Hijack are forwarded or SSE
// and streaming break.
type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	n, err := r.ResponseWriter.Write(b)
	r.bytes += n
	return n, err
}

func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// observe logs every request that did NOT go through cleanly, plus panics.
//
// Rationale: the handlers have ~25 distinct failure exits and only a handful
// logged anything, so "it just errors" was untraceable. Rather than sprinkle log
// calls on every branch (easy to miss one, and noisy to maintain), capture the
// outcome in one place:
//
//   - 5xx always logged (a bug or a broken disk, never routine)
//   - any 4xx on a mutating method logged (a push/delete that did not happen)
//   - panics logged with a stack and turned into 500 instead of killing the conn
//
// Routine 404s and 405 probes stay silent on purpose: they are the background
// radiation of anything reachable from the internet.
func observe(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w}
		defer func() {
			if p := recover(); p != nil {
				log.Printf("PANIC %s %s: %v\n%s", r.Method, r.URL.Path, p, debug.Stack())
				if rec.status == 0 {
					http.Error(rec, "Internal Server Error", http.StatusInternalServerError)
				}
				return
			}
			status := rec.status
			if status == 0 {
				status = http.StatusOK
			}
			mutating := r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodDelete
			if status >= 500 || (mutating && status >= 400) {
				log.Printf("REQ %s %s -> %d in %s (%d bytes) ua=%q",
					r.Method, r.URL.Path, status,
					time.Since(start).Round(time.Millisecond), rec.bytes,
					truncateUA(r.Header.Get("User-Agent")))
			}
		}()
		next.ServeHTTP(rec, r)
	})
}

func truncateUA(ua string) string {
	if len(ua) > 80 {
		return ua[:80] + "..."
	}
	return ua
}

// securityHeaders sets conservative defaults on every response. The app is a
// single self-contained page with no external resources, so a strict policy
// costs nothing and removes whole classes of injection risk.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
		// Cloudflare terminates TLS in front of us and forwards X-Forwarded-Proto.
		// Without HSTS the browser will happily follow an http:// link and send the
		// auth token — and the login password — in cleartext over the internet.
		if r.Header.Get("X-Forwarded-Proto") == "https" {
			h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
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
