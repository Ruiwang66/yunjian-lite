package shared

import (
	"strings"
	"testing"
)

func TestControlURL(t *testing.T) {
	for _, raw := range []string{"http://monitor.example", "http://192.0.2.1", "https://user:secret@monitor.example", "https://monitor.example?q=secret", "https://monitor.example/#fragment", "https://monitor.example/path", "https://monitor.example:70000", "https://monitor.example:0", "https://monitor.example?", "file:///tmp/key"} {
		if _, err := ControlURL(raw, false); err == nil {
			t.Errorf("accepted unsafe origin %q", raw)
		}
	}
	for _, raw := range []string{"https://monitor.example", "https://monitor.example:443/", "http://127.0.0.1:5050", "http://[::1]:5050", "http://localhost:5050"} {
		if _, err := ControlURL(raw, false); err != nil {
			t.Errorf("rejected safe origin %q: %v", raw, err)
		}
	}
	if _, err := ControlURL("http://monitor.example", true); err != nil {
		t.Fatal("explicit development override", err)
	}
	if !ValidKey(strings.Repeat("A", 43)) || ValidKey(strings.Repeat("A", 42)) || ValidKey(strings.Repeat("A", 42)+"\n") {
		t.Fatal("invalid key validation")
	}
}
