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
	"golang.org/x/crypto/bcrypt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAuthenticatedMonitoringLifecycle(t *testing.T) {
	dir := t.TempDir()
	db, err := database.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	created, err := service.BootstrapAdmin(db, dir, false)
	if err != nil || !created {
		t.Fatalf("bootstrap: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "admin-password.txt"))
	if err != nil {
		t.Fatal(err)
	}
	password := strings.TrimSpace(strings.Split(string(data), "password: ")[1])
	var u models.User
	db.First(&u)
	if bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(password)) != nil {
		t.Fatal("initial password is not a matching bcrypt hash")
	}
	if created, err := service.BootstrapAdmin(db, dir, false); created || err != nil {
		t.Fatal("restart must not reset admin")
	}
	cfg := config.Config{DataDir: dir, WebDir: dir, SessionTTL: time.Hour, OfflineAfter: 90 * time.Second}
	hub := live.New()
	a := New(db, cfg, hub)
	router := a.Router()
	var cookie *http.Cookie
	call := func(method, path string, body any, token, origin string) (int, map[string]any) {
		var payload []byte
		if body != nil {
			payload, _ = json.Marshal(body)
		}
		req := httptest.NewRequest(method, path, bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		if cookie != nil {
			req.AddCookie(cookie)
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if method == "POST" && path == "/api/auth/login" && w.Code == 200 {
			cookie = w.Result().Cookies()[0]
			if !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode {
				t.Fatal("unsafe session cookie")
			}
		}
		result := map[string]any{}
		_ = json.Unmarshal(w.Body.Bytes(), &result)
		return w.Code, result
	}
	if code, _ := call("GET", "/api/servers", nil, "", ""); code != 401 {
		t.Fatalf("private API: %d", code)
	}
	if code, _ := call("POST", "/api/auth/login", map[string]any{"username": "admin", "password": password}, "", "http://evil.example"); code != 403 {
		t.Fatal("cross-site login accepted")
	}
	if code, _ := call("POST", "/api/auth/login", map[string]any{"username": "admin", "password": password}, "", ""); code != 200 {
		t.Fatalf("login: %d", code)
	}
	code, result := call("POST", "/api/servers", map[string]any{"name": "Test Debian"}, "", "")
	if code != 201 {
		t.Fatalf("register: %d %v", code, result)
	}
	raw := result["token"].(string)
	var server models.Server
	db.First(&server)
	if server.Token == raw || len(server.Token) != 64 {
		t.Fatal("agent token not hashed")
	}
	payload := map[string]any{"cpu": 95, "memory": 40, "disk": 60, "memory_total": 1000, "memory_used": 400, "disk_total": 2000, "disk_used": 1200, "os": "linux", "platform": "debian", "ip": "192.0.2.1"}
	if code, _ := call("POST", "/api/agent/report", payload, "wrong-token", ""); code != 401 {
		t.Fatal("invalid Agent token accepted")
	}
	if code, result := call("POST", "/api/agent/report", payload, raw, ""); code != 200 {
		t.Fatalf("report: %d %v", code, result)
	}
	var logs []models.SystemLog
	db.Order("id asc").Find(&logs)
	if len(logs) != 2 || logs[0].Message != "Agent connected" || logs[1].Level != "WARN" {
		t.Fatalf("initial connection and warning missing: %+v", logs)
	}
	var freshCount int64
	if err := db.Model(&models.Server{}).Where("status = ? AND last_seen < ?", "online", time.Now().UTC().Add(-cfg.OfflineAfter)).Count(&freshCount).Error; err != nil || freshCount != 0 {
		t.Fatal("fresh UTC report incorrectly considered offline")
	}
	reqHistory := httptest.NewRequest("GET", "/api/servers/1/metrics?hours=6", nil)
	reqHistory.AddCookie(cookie)
	wHistory := httptest.NewRecorder()
	router.ServeHTTP(wHistory, reqHistory)
	var samples []models.Metric
	if json.Unmarshal(wHistory.Body.Bytes(), &samples) != nil || len(samples) != 1 {
		t.Fatal("UTC history sample missing: ", wHistory.Body.String())
	}
	if code, _ := call("POST", "/api/agent/report", payload, raw, ""); code != 429 {
		t.Fatal("rapid reports not limited")
	}
	db.Model(&server).Update("last_seen", time.Now().UTC().Add(-30*time.Second))
	if code, _ := call("POST", "/api/agent/report", payload, raw, ""); code != 200 {
		t.Fatal("second report failed")
	}
	var count int64
	db.Model(&models.SystemLog{}).Count(&count)
	if count != 2 {
		t.Fatalf("duplicate warnings: %d", count)
	}
	code, result = call("GET", "/api/servers/1", nil, "", "")
	if code != 200 || result["status"] != "online" || result["latest"] == nil {
		t.Fatalf("missing latest data: %v", result)
	}
	if _, ok := result["token"]; ok {
		t.Fatal("token leaked")
	}
	db.Model(&server).Update("last_seen", time.Now().UTC().Add(-100*time.Second))
	_, result = call("GET", "/api/servers/1", nil, "", "")
	if result["status"] != "offline" {
		t.Fatal("offline status not calculated")
	}
	if code, _ := call("POST", "/api/agent/report", payload, raw, ""); code != 200 {
		t.Fatal("reconnect failed")
	}
	db.Model(&models.SystemLog{}).Where("message = ?", "Agent connected").Count(&count)
	if count != 2 {
		t.Fatal("reconnect log missing")
	}
	code, result = call("POST", "/api/servers/1/token", nil, "", "")
	if code != 200 {
		t.Fatal("rotation failed")
	}
	newToken := result["token"].(string)
	if code, _ := call("POST", "/api/agent/report", payload, raw, ""); code != 401 {
		t.Fatal("rotated token still accepted")
	}
	db.Model(&server).Update("last_seen", time.Now().UTC().Add(-30*time.Second))
	payload["cpu"] = 30
	if code, _ := call("POST", "/api/agent/report", payload, newToken, ""); code != 200 {
		t.Fatal("new token not accepted")
	}
	if code, _ := call("GET", "/api/servers/1/metrics?hours=999", nil, "", ""); code != 400 {
		t.Fatal("unbounded history")
	}
	payload["cpu"] = 101
	if code, _ := call("POST", "/api/agent/report", payload, newToken, ""); code != 400 {
		t.Fatal("invalid percentages accepted")
	}
	// A real WebSocket receives change notifications and rejects a different origin.
	ts := httptest.NewServer(router)
	defer ts.Close()
	headers := http.Header{}
	headers.Add("Cookie", cookie.String())
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, headers)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, _, err := conn.ReadMessage(); err != nil {
		t.Fatal("WebSocket did not signal readiness: ", err)
	}
	hub.Broadcast("metrics_updated")
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg, err := conn.ReadMessage()
	if err != nil || !bytes.Contains(msg, []byte("metrics_updated")) {
		t.Fatalf("WebSocket update: %s %v", msg, err)
	}
	headers.Set("Origin", "http://evil.example")
	if other, _, err := websocket.DefaultDialer.Dial(wsURL, headers); err == nil {
		other.Close()
		t.Fatal("cross-origin WebSocket accepted")
	}
	if code, _ := call("POST", "/api/auth/password", map[string]any{"current_password": password, "new_password": "new-test-password-123"}, "", ""); code != 200 {
		t.Fatal("password change failed")
	}
	if code, _ := call("GET", "/api/auth/me", nil, "", ""); code != 401 {
		t.Fatal("old session not revoked")
	}
	if _, err := os.Stat(filepath.Join(dir, "admin-password.txt")); !os.IsNotExist(err) {
		t.Fatal("bootstrap password file not removed")
	}
	if code, _ := call("POST", "/api/auth/login", map[string]any{"username": "admin", "password": "new-test-password-123"}, "", ""); code != 200 {
		t.Fatal("new password does not work")
	}
	if code, _ := call("DELETE", "/api/servers/1", nil, "", ""); code != 200 {
		t.Fatal("delete failed")
	}
	db.Model(&models.Metric{}).Count(&count)
	if count != 0 {
		t.Fatal("orphan metrics remain")
	}
}
func TestLoginRateLimit(t *testing.T) {
	a := &API{attempts: map[string]attempt{}}
	for i := 0; i < 10; i++ {
		if !a.allowLogin("192.0.2.2") {
			t.Fatal("premature lockout")
		}
	}
	if a.allowLogin("192.0.2.2") {
		t.Fatal("login rate limit missing")
	}
	a.attempts["192.0.2.2"] = attempt{Count: 10, Until: time.Now().Add(-time.Minute)}
	if !a.allowLogin("192.0.2.2") {
		t.Fatal("expired limit not cleared")
	}
}
