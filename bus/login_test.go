package bus

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func loginRequest(remoteIP, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("POST", "/api/login", strings.NewReader(body))
	r.RemoteAddr = remoteIP
	w := httptest.NewRecorder()
	handleLogin(w, r)
	return w
}

func resetLoginState() {
	attemptsMu.Lock()
	ipAttempts = make(map[string]IPAttempt)
	globalAttempt = IPAttempt{}
	attemptsMu.Unlock()
}

func TestGlobalBackoffBlocksFreshIP(t *testing.T) {
	oldConfig := config
	config = Config{Token: "tok", Password: "pw", LoginKey: "key"}
	defer func() { config = oldConfig }()
	resetLoginState()

	if w := loginRequest("1.2.3.4:1111", `{"password":"x","key":"y"}`); w.Code != 401 {
		t.Fatalf("first wrong attempt: want 401, got %d", w.Code)
	}
	// A different IP, never seen before, must still hit the global brake.
	if w := loginRequest("9.9.9.9:2222", `{"password":"x","key":"y"}`); w.Code != 429 {
		t.Fatalf("fresh IP after global failure: want 429, got %d", w.Code)
	}
}

func TestLoginSuccessReleasesGlobalBrake(t *testing.T) {
	oldConfig := config
	config = Config{Token: "tok", Password: "pw", LoginKey: "key"}
	defer func() { config = oldConfig }()
	resetLoginState()

	loginRequest("1.2.3.4:1111", `{"password":"x","key":"y"}`) // arms the global brake

	w := loginRequest("5.6.7.8:3333", `{"password":"pw","key":"key"}`)
	if w.Code != 200 {
		t.Fatalf("correct login under brake: want 200, got %d (%s)", w.Code, w.Body.String())
	}
	attemptsMu.Lock()
	g := globalAttempt
	attemptsMu.Unlock()
	if g.FailCount != 0 {
		t.Fatalf("success must clear global counter, got %d", g.FailCount)
	}
	// And a fresh wrong attempt from yet another IP starts clean at 401 again.
	if w := loginRequest("7.7.7.7:4444", `{"password":"x","key":"y"}`); w.Code != 401 {
		t.Fatalf("post-success wrong attempt: want 401, got %d", w.Code)
	}
}
