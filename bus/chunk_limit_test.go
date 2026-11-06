package bus

import (
	"fmt"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// files/.chunks is excluded from the rolling quota on purpose, so nothing else
// bounds it. Without a cap a client can open unlimited 500 MB slots and fill
// the disk. Guard both dimensions: session count and declared bytes.
func TestChunkInitIsBounded(t *testing.T) {
	tmp := t.TempDir()
	oldFilesDir := filesDir
	filesDir = tmp
	defer func() { filesDir = oldFilesDir }()

	uploadsMu.Lock()
	uploads = map[string]*chunkSession{}
	uploadsMu.Unlock()

	initOne := func(size int64) int {
		body := fmt.Sprintf(`{"filename":"x.bin","size":%d,"device":"pc"}`, size)
		w := httptest.NewRecorder()
		handleChunkInit(w, httptest.NewRequest("POST", "/api/push/chunk/init", strings.NewReader(body)))
		return w.Code
	}

	// Fill up the session slots.
	for i := 0; i < maxConcurrentUploads; i++ {
		if code := initOne(1024); code != 201 {
			t.Fatalf("init %d: want 201, got %d", i, code)
		}
	}
	if code := initOne(1024); code != 429 {
		t.Fatalf("beyond maxConcurrentUploads: want 429, got %d", code)
	}

	// Reset (and clear the dirs from phase 1) to verify the byte ceiling.
	uploadsMu.Lock()
	uploads = map[string]*chunkSession{}
	uploadsMu.Unlock()
	os.RemoveAll(chunksRoot())
	os.MkdirAll(chunksRoot(), 0755)

	oldPending := maxPendingBytes
	maxPendingBytes = 3 << 20 // 3 MiB
	defer func() { maxPendingBytes = oldPending }()

	if code := initOne(2 << 20); code != 201 {
		t.Fatalf("2 MiB: want 201, got %d", code)
	}
	if code := initOne(2 << 20); code != 429 {
		t.Fatalf("2 MiB over a 3 MiB budget: want 429, got %d", code)
	}

	// Rejected sessions must not leave temp dirs behind.
	entries, _ := os.ReadDir(chunksRoot())
	if len(entries) > maxConcurrentUploads {
		t.Errorf("orphaned temp dirs: %d", len(entries))
	}
}
