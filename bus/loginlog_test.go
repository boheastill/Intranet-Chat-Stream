package bus

import (
	"bytes"
	"log"
	"net/http/httptest"
	"strings"
	"testing"
)

// Every login outcome must leave exactly one log line. The old code logged only
// 401 and 200, so rate-limited and malformed attempts — the two most common when
// a user reports "it just errors" — produced nothing at all.
func TestEveryLoginOutcomeIsLogged(t *testing.T) {
	oldConfig := config
	config = Config{Token: "tok", Password: "pw12", LoginKey: "key1"}
	defer func() { config = oldConfig }()

	cases := []struct {
		name    string
		wire    func(*httptest.ResponseRecorder) *httptest.ResponseRecorder
		body    string
		method  string
		wantLog string
	}{
		{"wrong GET", nil, "", "GET", "LOGIN REJECTED"},
		{"malformed body", nil, "{not json", "POST", "LOGIN REJECTED"},
		{"empty password", nil, `{"password":"","key":"key1"}`, "POST", "LOGIN REJECTED"},
		{"empty key", nil, `{"password":"pw12","key":""}`, "POST", "LOGIN REJECTED"},
		{"wrong password", nil, `{"password":"nope","key":"key1"}`, "POST", "LOGIN FAILED"},
		{"wrong key", nil, `{"password":"pw12","key":"nope"}`, "POST", "LOGIN FAILED"},
		{"success", nil, `{"password":"pw12","key":"key1"}`, "POST", "LOGIN SUCCESS"},
	}

	for _, c := range cases {
		resetLoginState()

		var buf bytes.Buffer
		log.SetOutput(&buf)
		req := httptest.NewRequest(c.method, "/api/login", strings.NewReader(c.body))
		req.RemoteAddr = "1.2.3.4:1111"
		req.Header.Set("User-Agent", "test-agent")
		handleLogin(httptest.NewRecorder(), req)
		log.SetOutput(log.Writer())

		if !strings.Contains(buf.String(), c.wantLog) {
			t.Errorf("%s: log must contain %q, got %q", c.name, c.wantLog, buf.String())
		}
		if !strings.Contains(buf.String(), "ua=") {
			t.Errorf("%s: log must record the user agent, got %q", c.name, buf.String())
		}
	}

	// Rate limiting must be visible too.
	resetLoginState()
	loginRequest("5.6.7.8:2222", `{"password":"x","key":"y"}`) // arms the global brake
	var buf bytes.Buffer
	log.SetOutput(&buf)
	loginRequest("5.6.7.8:2222", `{"password":"x","key":"y"}`) // now blocked
	log.SetOutput(log.Writer())
	if !strings.Contains(buf.String(), "RATE-LIMITED") {
		t.Errorf("rate-limited attempt must be logged, got %q", buf.String())
	}
}

// The log line must help diagnose a full-width input mistake: char count vs
// byte count differ, which is the signature of ９９９９ instead of 9999.
func TestFailureLogShowsCharAndByteCounts(t *testing.T) {
	oldConfig := config
	config = Config{Token: "tok", Password: "9999", LoginKey: "fakar"}
	defer func() { config = oldConfig }()
	resetLoginState()

	var buf bytes.Buffer
	log.SetOutput(&buf)
	loginRequest("9.9.9.9:1", `{"password":"９９９９","key":"fakar"}`)
	log.SetOutput(log.Writer())

	line := buf.String()
	if !strings.Contains(line, "chars=4") || !strings.Contains(line, "bytes=12") {
		t.Fatalf("log must expose the char/byte mismatch, got %q", line)
	}
	if !strings.Contains(line, "keyOK=true") {
		t.Fatalf("log must show the key matched, got %q", line)
	}
}
