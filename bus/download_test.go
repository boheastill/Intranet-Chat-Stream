package bus

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Uploaded files live on the same origin as the app. If /api/download/ ever
// returns text/html (or any sniffable type) the browser will render it and any
// script inside runs with the app's origin — enough to read the auth token out
// of localStorage. This is the regression guard for that.
func TestDownloadIsNeverRenderable(t *testing.T) {
	tmp := t.TempDir()
	oldFilesDir := filesDir
	filesDir = tmp
	defer func() { filesDir = oldFilesDir }()

	name := "1700000000_pc_evil.html"
	if err := os.WriteFile(filepath.Join(tmp, name), []byte("<script>alert(1)</script>"), 0644); err != nil {
		t.Fatal(err)
	}

	for _, query := range []string{"", "?download=1"} {
		w := httptest.NewRecorder()
		handleDownload(w, httptest.NewRequest("GET", "/api/download/"+name+query, nil))

		if w.Code != 200 {
			t.Fatalf("%q: want 200, got %d", query, w.Code)
		}
		if ct := w.Header().Get("Content-Type"); ct != "application/octet-stream" {
			t.Errorf("%q: Content-Type must be application/octet-stream, got %q", query, ct)
		}
		csp := w.Header().Get("Content-Security-Policy")
		if !strings.Contains(csp, "sandbox") {
			t.Errorf("%q: CSP must sandbox the response, got %q", query, csp)
		}
	}
}
