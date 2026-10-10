// Package websocket sends bounded notifications, never client-supplied commands.
package websocket

import (
	"context"
	"encoding/json"
	"github.com/gorilla/websocket"
	"net"
	"net/http"
	"sync"
	"time"
)

type client struct {
	send chan []byte
	stop chan struct{}
	key  string
}
type Hub struct {
	mu                        sync.Mutex
	clients                   map[*client]struct{}
	counts                    map[string]int
	total, maxTotal, maxPerIP int
	closed                    bool
}

func New() *Hub {
	return &Hub{clients: map[*client]struct{}{}, counts: map[string]int{}, maxTotal: 256, maxPerIP: 8}
}
func (h *Hub) Broadcast(kind string) {
	payload, _ := json.Marshal(map[string]any{"type": kind, "at": time.Now().UTC()})
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return
	}
	for c := range h.clients {
		select {
		case c.send <- payload:
		default:
		}
	}
}
func (h *Hub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return
	}
	h.closed = true
	for c := range h.clients {
		close(c.stop)
	}
}
func (h *Hub) reserve(key string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed || h.total >= h.maxTotal || h.counts[key] >= h.maxPerIP {
		return false
	}
	h.total++
	h.counts[key]++
	return true
}
func (h *Hub) release(c *client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.clients, c)
	h.total--
	h.counts[c.key]--
	if h.counts[c.key] == 0 {
		delete(h.counts, c.key)
	}
}

// Authenticated handlers check sessions before upgrade; the default origin
// check rejects a different browser host. The API also compares the scheme.
func (h *Hub) Serve(w http.ResponseWriter, r *http.Request, expires time.Time, authValid func() bool, sourceIP ...string) {
	key, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		key = r.RemoteAddr
	}
	if len(sourceIP) == 1 && net.ParseIP(sourceIP[0]) != nil {
		key = sourceIP[0]
	}
	if !h.reserve(key) {
		w.Header().Set("Retry-After", "25")
		http.Error(w, "WebSocket connection limit reached", 429)
		return
	}
	c := &client{send: make(chan []byte, 8), stop: make(chan struct{}), key: key}
	defer h.release(c)
	upgrader := websocket.Upgrader{ReadBufferSize: 1024, WriteBufferSize: 1024}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return
	}
	h.clients[c] = struct{}{}
	c.send <- []byte(`{"type":"connected"}`)
	h.mu.Unlock()
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	conn.SetReadLimit(1024)
	conn.SetReadDeadline(time.Now().Add(70 * time.Second))
	conn.SetPongHandler(func(string) error { return conn.SetReadDeadline(time.Now().Add(70 * time.Second)) })
	go func() {
		defer cancel()
		if _, _, err := conn.ReadMessage(); err == nil {
			_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "notification-only connection"), time.Now().Add(time.Second))
		}
	}()
	timer := time.NewTimer(time.Until(expires))
	defer timer.Stop()
	ticker := time.NewTicker(25 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-c.stop:
			return
		case <-timer.C:
			return
		case msg := <-c.send:
			if err := conn.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
				return
			}
			if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-ticker.C:
			if !authValid() {
				return
			}
			if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second)); err != nil {
				return
			}
		}
	}
}
