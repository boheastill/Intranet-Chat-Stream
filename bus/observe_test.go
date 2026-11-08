package bus

import (
	"bytes"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func captureLog(fn func()) string {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	fn()
	log.SetOutput(log.Writer())
	return buf.String()
}

func serve(h http.Handler, method, path string) {
	req := httptest.NewRequest(method, path, nil)
	req.Header.Set("User-Agent", "test-agent")
	h.ServeHTTP(httptest.NewRecorder(), req)
}

// A failing write is exactly the case a user reports as "it did not send" and an
// operator needs in the journal. Previously ~25 failure exits logged nothing.
func TestObserveLogsFailuresOnMutations(t *testing.T) {
	h := observe(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "disk full", http.StatusInternalServerError)
	}))

	out := captureLog(func() { serve(h, "POST", "/api/push") })
	if !strings.Contains(out, "REQ POST /api/push -> 500") {
		t.Fatalf("5xx on a mutation must be logged, got %q", out)
	}
	if !strings.Contains(out, "test-agent") {
		t.Fatalf("log should include the user agent, got %q", out)
	}

	// A 4xx on a mutation is also a write that did not happen.
	h2 := observe(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad", http.StatusBadRequest)
	}))
	out = captureLog(func() { serve(h2, "DELETE", "/api/action") })
	if !strings.Contains(out, "REQ DELETE /api/action -> 400") {
		t.Fatalf("4xx on a mutation must be logged, got %q", out)
	}
}

// Routine internet noise must not fill the journal: a self-hosted box receives
// constant 404/405 probing, and logging it would recreate the log-growth problem.
func TestObserveStaysQuietOnRoutineNoise(t *testing.T) {
	h := observe(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusNotFound)
	}))
	out := captureLog(func() {
		serve(h, "GET", "/nope")
		serve(h, "GET", "/api/list")
	})
	if out != "" {
		t.Fatalf("404 on GET must stay silent, got %q", out)
	}

	h2 := observe(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	}))
	out = captureLog(func() { serve(h2, "POST", "/api/push") })
	if out != "" {
		t.Fatalf("successful request must stay silent, got %q", out)
	}
}

// A handler panic must be logged and answered with 500, not tear down the
// connection or the process.
func TestObserveRecoversPanics(t *testing.T) {
	h := observe(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("boom")
	}))
	var got int
	out := captureLog(func() {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("POST", "/api/push", nil))
		got = rec.Code
	})
	if got != http.StatusInternalServerError {
		t.Fatalf("panic should become 500, got %d", got)
	}
	if !strings.Contains(out, "PANIC POST /api/push") || !strings.Contains(out, "boom") {
		t.Fatalf("panic must be logged with its message, got %q", out)
	}
	if !strings.Contains(out, "goroutine") {
		t.Fatalf("panic log should include a stack, got %q", out)
	}
}

// SSE depends on the wrapped writer still exposing Flush.
func TestObservePreservesFlusher(t *testing.T) {
	h := observe(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := w.(http.Flusher); !ok {
			t.Error("wrapped writer lost http.Flusher — SSE would break")
			return
		}
		w.(http.Flusher).Flush()
	}))
	serve(h, "GET", "/api/stream")
}
