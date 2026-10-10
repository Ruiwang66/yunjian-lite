package api

import (
	"bytes"
	"cloudease-lite/server/config"
	"cloudease-lite/server/database"
	"cloudease-lite/server/models"
	"cloudease-lite/server/service"
	live "cloudease-lite/server/websocket"
	"encoding/json"
	"github.com/gorilla/websocket"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPublicMonitoringAndFourSecondReports(t *testing.T) {
	db, err := database.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	hub := live.New()
	router := New(db, config.Config{OfflineAfter: 90 * time.Second}, hub).Router()
	now := time.Now().UTC()
	last := now.Add(-4100 * time.Millisecond)
	s := models.Server{Name: "Public server", Token: service.HashToken("test-token"), IP: "192.0.2.1", Status: "online", LastSeen: &last}
	if err := db.Create(&s).Error; err != nil {
		t.Fatal(err)
	}
	// More than the old 3000-row cap: both ends of the 24-hour window must survive.
	rows := make([]models.Metric, 3600)
	for i := range rows {
		rows[i] = models.Metric{ServerID: s.ID, CPU: 25, CreatedAt: now.Add(-23 * time.Hour).Add(time.Duration(i) * 23 * time.Second)}
	}
	if err := db.CreateInBatches(rows, 200).Error; err != nil {
		t.Fatal(err)
	}
	call := func(method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer test-token")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}
	w := call("GET", "/api/public/servers/1", "")
	if w.Code != 200 {
		t.Fatalf("public detail: %d", w.Code)
	}
	var view map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"ip", "token", "password", "created_at"} {
		if _, ok := view[key]; ok {
			t.Fatalf("private field leaked: %s", key)
		}
	}
	if view["name"] != "Public server" || view["latest"] == nil {
		t.Fatal("public metrics missing")
	}
	w = call("GET", "/api/public/servers", "")
	if w.Code != 200 || !bytes.Contains(w.Body.Bytes(), []byte("Public server")) {
		t.Fatal("public list missing")
	}
	w = call("GET", "/api/public/servers/1/metrics?hours=24", "")
	var samples []models.Metric
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &samples) != nil || len(samples) < 2 || len(samples) > 1201 {
		t.Fatalf("history sampling: %d %d %s", w.Code, len(samples), w.Body.String())
	}
	if now.Sub(samples[0].CreatedAt) < 22*time.Hour || now.Sub(samples[len(samples)-1].CreatedAt) > time.Minute {
		t.Fatal("history window truncated")
	}
	for _, path := range []string{"/api/servers", "/api/logs", "/api/downloads/cloud-agent.exe"} {
		if call("GET", path, "").Code != 401 {
			t.Fatalf("private API exposed: %s", path)
		}
	}
	if call("POST", "/api/public/servers", "{}").Code != 404 || call("POST", "/api/servers", "{}").Code != 401 {
		t.Fatal("public write accepted")
	}
	w = call("POST", "/api/agent/report", `{"cpu":25,"memory":30,"disk":40}`)
	if w.Code != 200 || !bytes.Contains(w.Body.Bytes(), []byte(`"next_report_seconds":4`)) {
		t.Fatalf("4-second report rejected: %s", w.Body.String())
	}
	if call("POST", "/api/agent/report", `{"cpu":25,"memory":30,"disk":40}`).Code != 429 {
		t.Fatal("rapid report accepted")
	}
	ts := httptest.NewServer(router)
	defer ts.Close()
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(ts.URL, "http")+"/api/public/ws", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, msg, err := conn.ReadMessage(); err != nil || !bytes.Contains(msg, []byte("connected")) {
		t.Fatal("public websocket not connected")
	}
	hub.Broadcast("metrics_updated")
	if _, msg, err := conn.ReadMessage(); err != nil || !bytes.Contains(msg, []byte("metrics_updated")) {
		t.Fatal("public websocket update missing")
	}
}
