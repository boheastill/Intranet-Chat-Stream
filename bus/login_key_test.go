package bus

import "testing"

// The login has two independent factors: the URL key and the password. Both are
// deliberately short enough to type from memory on a fresh device, so neither
// may be dropped — the strength comes from the product of their search spaces.
func TestLoginRequiresBothFactors(t *testing.T) {
	oldConfig := config
	config = Config{Token: "tok", Password: "83007311", LoginKey: "mango"}
	defer func() { config = oldConfig }()

	ok := []struct{ name, body string }{
		{"both correct", `{"password":"83007311","key":"mango"}`},
		{"key uppercase", `{"password":"83007311","key":"MANGO"}`},
		{"key mixed case", `{"password":"83007311","key":"ManGo"}`},
		{"key padded", `{"password":"83007311","key":"  mango  "}`},
	}
	for _, c := range ok {
		resetLoginState()
		if w := loginRequest("1.2.3.4:1111", c.body); w.Code != 200 {
			t.Errorf("%s: want 200, got %d (%s)", c.name, w.Code, w.Body.String())
		}
	}

	bad := []struct{ name, body string }{
		{"wrong key", `{"password":"83007311","key":"wrong"}`},
		{"empty key", `{"password":"83007311","key":""}`},
		{"missing key", `{"password":"83007311"}`},
		{"wrong password", `{"password":"00000000","key":"mango"}`},
		{"both wrong", `{"password":"00000000","key":"wrong"}`},
	}
	for _, c := range bad {
		resetLoginState()
		// Empty/missing fields are rejected as a bad payload (400); a mismatch is
		// 401. Either way the request must never yield a token.
		if w := loginRequest("1.2.3.4:1111", c.body); w.Code != 401 && w.Code != 400 {
			t.Errorf("%s: want 400 or 401, got %d", c.name, w.Code)
		}
	}
}
