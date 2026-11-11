package websocket

import (
	"encoding/json"
	"github.com/gorilla/websocket"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestV05PublicSocketDoesNotReceivePrivateActivity(t *testing.T) {
	h := New()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.ServePublic(w, r, time.Now().Add(time.Hour), "192.0.2.1")
	}))
	defer ts.Close()
	defer h.Close()
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(ts.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, _, err := conn.ReadMessage(); err != nil {
		t.Fatal(err)
	}
	h.Broadcast("security_changed")
	h.Broadcast("metrics_updated")
	var msg struct {
		Type string `json:"type"`
	}
	if err := conn.ReadJSON(&msg); err != nil || msg.Type != "metrics_updated" {
		t.Fatalf("private activity reached public socket: %+v %v", msg, err)
	}
}

func TestV05AuthenticatedUsersBehindNATHaveIndependentSocketSlots(t *testing.T) {
	h := New()
	h.maxPerSource = 1
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.Serve(w, r, time.Now().Add(time.Hour), func() bool { return true }, "192.0.2.1", "user:"+r.URL.Query().Get("fixture_user"))
	}))
	defer ts.Close()
	defer h.Close()
	var conns []*websocket.Conn
	defer func() {
		for _, c := range conns {
			c.Close()
		}
	}()
	for _, id := range []string{"1", "2"} {
		conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(ts.URL, "http")+"?fixture_user="+id, nil)
		if err != nil {
			t.Fatal("NAT user rejected", err)
		}
		conns = append(conns, conn)
		conn.SetReadDeadline(time.Now().Add(time.Second))
		var msg map[string]json.RawMessage
		if err := conn.ReadJSON(&msg); err != nil {
			t.Fatal(err)
		}
	}
}

func TestV05RevokedSessionStopsNotificationsImmediately(t *testing.T) {
	h := New()
	var valid atomic.Bool
	valid.Store(true)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.Serve(w, r, time.Now().Add(time.Hour), valid.Load)
	}))
	defer ts.Close()
	defer h.Close()
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(ts.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, _, err = conn.ReadMessage(); err != nil {
		t.Fatal(err)
	}
	valid.Store(false)
	h.Broadcast("private-update")
	_, msg, err := conn.ReadMessage()
	if err == nil {
		t.Fatalf("notification sent after revocation: %s", msg)
	}
	if e, ok := err.(interface{ Timeout() bool }); ok && e.Timeout() {
		t.Fatal("revoked connection left open until heartbeat")
	}
}
