package bus

import (
	"crypto/subtle"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// IPAttempt tracks failed login attempts for exponential backoff per IP.
type IPAttempt struct {
	FailCount int
	LastTime  time.Time
}

var (
	attemptsMu sync.Mutex
	ipAttempts = make(map[string]IPAttempt)
	// globalAttempt applies the same exponential brake to every login
	// regardless of source — the countermeasure that still bites when an
	// attacker rotates IPs faster than per-IP tracking can throttle them.
	globalAttempt IPAttempt
)

// tokenAuthMiddleware enforces secret Token checks for all private API routes.
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

		if subtle.ConstantTimeCompare([]byte(token), []byte(secretToken)) != 1 {
			log.Printf("[%s] Blocked Unauthorized request: %s %s (Token mismatch)", r.RemoteAddr, r.Method, r.URL.Path)
			http.Error(w, "Unauthorized: Invalid or missing token", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// getClientIP extracts client IP address taking into account Cloudflare headers.
func getClientIP(r *http.Request) string {
	if cfIP := r.Header.Get("CF-Connecting-IP"); cfIP != "" {
		return cfIP
	}
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}
	// net.SplitHostPort handles IPv6 literals ("[2406:da18::1]:443"); the old
	// strings.Split(":") approach returned just "2406" for those, which made every
	// IPv6 client share one backoff bucket.
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}
