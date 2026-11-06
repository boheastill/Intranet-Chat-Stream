package bus

import (
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestGetChannelDirRejectsTraversal(t *testing.T) {
	bad := []string{"..", ".", "...", "a/b", "a\\b", ".chunks", "a b"}
	for _, c := range bad {
		r := httptest.NewRequest("GET", "/api/list?channel="+url.QueryEscape(c), nil)
		_, dir := getChannelDir(r)
		if dir != filesDir {
			t.Errorf("channel %q: want %q, got %q", c, filesDir, dir)
		}
	}
	good := []string{"work", "client-work", "lab_1", "a.b"}
	for _, c := range good {
		r := httptest.NewRequest("GET", "/api/list?channel="+url.QueryEscape(c), nil)
		name, _ := getChannelDir(r)
		if name != c {
			t.Errorf("channel %q: want name %q, got %q", c, c, name)
		}
	}
}
