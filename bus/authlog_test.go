package bus

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// A client stuck on a stale token polls forever. Logging every attempt produced
// ~900 lines/day per client, which over a multi-year run is the main way this
// service's journal grows. Only the first failure per source should be logged.
func TestAuthFailureLogIsDeduplicated(t *testing.T) {
	authLogMu.Lock()
	authLogged = make(map[string]struct{})
	authLogMu.Unlock()

	if !shouldLogAuthFailure("1.2.3.4:1111", "GET", "/api/list") {
		t.Fatal("first failure from a source must be logged")
	}
	for i := 0; i < 100; i++ {
		if shouldLogAuthFailure("1.2.3.4:1111", "GET", "/api/list") {
			t.Fatal("repeat failures from the same source must be silenced")
		}
	}
	if !shouldLogAuthFailure("5.6.7.8:2222", "GET", "/api/list") {
		t.Fatal("a different source must still be logged")
	}

	// Once the source authenticates, a later failure is news again.
	noteAuthSuccess("1.2.3.4:1111")
	if !shouldLogAuthFailure("1.2.3.4:1111", "GET", "/api/list") {
		t.Fatal("after a success the next failure must be logged again")
	}

	// The set must stay bounded no matter how many sources appear.
	authLogMu.Lock()
	authLogged = make(map[string]struct{})
	authLogMu.Unlock()
	for i := 0; i < authLogMax*4; i++ {
		shouldLogAuthFailure(string(rune('a'+i%26))+string(rune('0'+i%10))+":1", "GET", "/")
	}
	authLogMu.Lock()
	n := len(authLogged)
	authLogMu.Unlock()
	if n > authLogMax {
		t.Fatalf("auth log set must stay bounded, got %d > %d", n, authLogMax)
	}
}

// The middleware must still reject bad tokens and accept good ones.
func TestAuthMiddlewareStillEnforces(t *testing.T) {
	authLogMu.Lock()
	authLogged = make(map[string]struct{})
	authLogMu.Unlock()

	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	h := tokenAuthMiddleware(ok, "secret")

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/list", nil))
	if w.Code != 401 {
		t.Fatalf("missing token: want 401, got %d", w.Code)
	}

	w = httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/list", nil)
	r.Header.Set("X-Auth-Token", "secret")
	h.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("valid token: want 200, got %d", w.Code)
	}
}
