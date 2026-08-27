package bus

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestChunkUploadLifecycle(t *testing.T) {
	tmp := t.TempDir()
	oldFilesDir := filesDir
	filesDir = tmp
	defer func() { filesDir = oldFilesDir }()
	if err := os.MkdirAll(chunksRoot(), 0755); err != nil {
		t.Fatal(err)
	}

	// init
	initBody := `{"filename":"holiday video.mp4","size":5,"device":"mobile"}`
	w := httptest.NewRecorder()
	handleChunkInit(w, httptest.NewRequest("POST", "/api/push/chunk/init", strings.NewReader(initBody)))
	if w.Code != 201 {
		t.Fatalf("init: want 201, got %d (%s)", w.Code, w.Body.String())
	}
	var initResp struct {
		UploadID string `json:"upload_id"`
	}
	json.Unmarshal(w.Body.Bytes(), &initResp)
	if len(initResp.UploadID) != 32 {
		t.Fatalf("init: bad upload id %q", initResp.UploadID)
	}

	// status mid-flight
	w = httptest.NewRecorder()
	handleChunkStatus(w, httptest.NewRequest("GET", "/api/push/chunk/status?id="+initResp.UploadID, nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"written":0`) {
		t.Fatalf("status: want written=0, got %d %s", w.Code, w.Body.String())
	}

	// data append
	w = httptest.NewRecorder()
	handleChunkData(w, httptest.NewRequest("PUT",
		"/api/push/chunk/data/"+initResp.UploadID+"?offset=0", bytes.NewReader([]byte("hello"))))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"written":5`) {
		t.Fatalf("data: want written=5, got %d %s", w.Code, w.Body.String())
	}

	// wrong offset rejected (no byte lost)
	w = httptest.NewRecorder()
	handleChunkData(w, httptest.NewRequest("PUT",
		"/api/push/chunk/data/"+initResp.UploadID+"?offset=3", bytes.NewReader([]byte("lo"))))
	if w.Code != 409 {
		t.Fatalf("data mismatched offset: want 409, got %d %s", w.Code, w.Body.String())
	}

	// complete promotes into the live store like a direct push would
	w = httptest.NewRecorder()
	handleChunkComplete(w, httptest.NewRequest("POST", "/api/push/chunk/complete",
		strings.NewReader(`{"id":"`+initResp.UploadID+`"}`)))
	if w.Code != 201 {
		t.Fatalf("complete: want 201, got %d (%s)", w.Code, w.Body.String())
	}
	var doneResp struct {
		ID string `json:"id"`
	}
	json.Unmarshal(w.Body.Bytes(), &doneResp)
	wantSuffix := "_mobile_holiday_video.mp4"
	if !strings.HasSuffix(doneResp.ID, wantSuffix) {
		t.Fatalf("complete: id %q lacks suffix %q", doneResp.ID, wantSuffix)
	}
	final, err := os.ReadFile(filepath.Join(tmp, doneResp.ID))
	if err != nil || string(final) != "hello" {
		t.Fatalf("final file read: err=%v content=%q", err, final)
	}
	// temp dir cleaned up after completion
	if _, err := os.Stat(filepath.Join(chunksRoot(), initResp.UploadID)); !os.IsNotExist(err) {
		t.Fatalf("temp dir still present after complete")
	}
}

func TestCompleteRejectsShortUpload(t *testing.T) {
	tmp := t.TempDir()
	oldFilesDir := filesDir
	filesDir = tmp
	defer func() { filesDir = oldFilesDir }()

	w := httptest.NewRecorder()
	handleChunkInit(w, httptest.NewRequest("POST", "/api/push/chunk/init",
		strings.NewReader(`{"filename":"a.bin","size":10,"device":"pc"}`)))
	var initResp struct {
		UploadID string `json:"upload_id"`
	}
	json.Unmarshal(w.Body.Bytes(), &initResp)

	handleChunkData(httptest.NewRecorder(), httptest.NewRequest("PUT",
		"/api/push/chunk/data/"+initResp.UploadID+"?offset=0", bytes.NewReader([]byte("12345"))))

	w = httptest.NewRecorder()
	handleChunkComplete(w, httptest.NewRequest("POST", "/api/push/chunk/complete",
		strings.NewReader(`{"id":"`+initResp.UploadID+`"}`)))
	if w.Code != 409 {
		t.Fatalf("complete short upload: want 409, got %d %s", w.Code, w.Body.String())
	}
}

func TestOvershootCannotPushPastDeclaredSize(t *testing.T) {
	tmp := t.TempDir()
	oldFilesDir := filesDir
	filesDir = tmp
	defer func() { filesDir = oldFilesDir }()
	os.MkdirAll(chunksRoot(), 0755)

	w := httptest.NewRecorder()
	handleChunkInit(w, httptest.NewRequest("POST", "/api/push/chunk/init",
		strings.NewReader(`{"filename":"b.bin","size":6,"device":"pc"}`)))
	var initResp struct {
		UploadID string `json:"upload_id"`
	}
	json.Unmarshal(w.Body.Bytes(), &initResp)

	// Shove MORE bytes than the session expects in one request.
	w = httptest.NewRecorder()
	handleChunkData(w, httptest.NewRequest("PUT",
		"/api/push/chunk/data/"+initResp.UploadID+"?offset=0", bytes.NewReader([]byte("12345678"))))
	if w.Code != 400 {
		t.Fatalf("overshoot append: want 400, got %d %s", w.Code, w.Body.String())
	}
	uploadsMu.Lock()
	got := uploads[initResp.UploadID].Written
	uploadsMu.Unlock()
	if got != 6 {
		t.Fatalf("written must clamp to declared size, got %d", got)
	}
	// Once full, any further data request is a conflict — never past the end.
	w = httptest.NewRecorder()
	handleChunkData(w, httptest.NewRequest("PUT",
		"/api/push/chunk/data/"+initResp.UploadID+"?offset=0", bytes.NewReader([]byte("x"))))
	if w.Code != 409 {
		t.Fatalf("append after full: want 409, got %d %s", w.Code, w.Body.String())
	}
}
