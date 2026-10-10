package websocket

import (
	"github.com/gorilla/websocket"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestConnectionLimitAndNotificationOnly(t *testing.T) {
	h := New()
	h.maxTotal = 2
	h.maxPerIP = 1
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.Serve(w, r, time.Now().Add(time.Hour), func() bool { return true })
	}))
	defer ts.Close()
	defer h.Close()
	url := "ws" + strings.TrimPrefix(ts.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, _, err = conn.ReadMessage(); err != nil {
		t.Fatal(err)
	}
	other, response, err := websocket.DefaultDialer.Dial(url, nil)
	if err == nil {
		other.Close()
		t.Fatal("per-IP connection cap bypass")
	}
	if response == nil || response.StatusCode != 429 {
		t.Fatal("expected limit rejection")
	}
	if err := conn.WriteMessage(websocket.TextMessage, []byte("client command")); err != nil {
		t.Fatal(err)
	}
	_, _, err = conn.ReadMessage()
	if !websocket.IsCloseError(err, websocket.ClosePolicyViolation) {
		t.Fatalf("client message not closed: %v", err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		h.mu.Lock()
		n := h.total
		h.mu.Unlock()
		if n == 0 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	h.mu.Lock()
	n := h.total
	h.mu.Unlock()
	if n != 0 {
		t.Fatal("slot leaked")
	}
	h.Close()
	if h.reserve("another") {
		t.Fatal("closed hub accepts connection")
	}
}
func TestTrustedSourceCapsAndFailedUpgradeRelease(t *testing.T) {
	h := New()
	h.maxTotal = 2
	h.maxPerIP = 1
	for i := 0; i < 3; i++ {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "http://example.com", nil)
		h.Serve(w, r, time.Now().Add(time.Hour), func() bool { return true }, "192.0.2.1")
		if w.Code == 429 {
			t.Fatal("failed upgrade leaked slot")
		}
	}
	if !h.reserve("one") || !h.reserve("two") || h.reserve("three") {
		t.Fatal("total cap failed")
	}
	h.release(&client{key: "one"})
	h.release(&client{key: "two"})
	h.Close()
}
