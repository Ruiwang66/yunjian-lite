package api

import (
	"cloudease-lite/server/config"
	"cloudease-lite/server/database"
	"cloudease-lite/server/models"
	"cloudease-lite/server/service"
	live "cloudease-lite/server/websocket"
	"cloudease-lite/shared"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSecurityValidationAndReplay(t *testing.T) {
	dir := t.TempDir()
	db, err := database.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	user := models.User{Username: "fixture", Password: "unused"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	session := models.Session{UserID: user.ID, TokenHash: service.HashToken("session-fixture"), ExpiresAt: time.Now().Add(time.Hour)}
	if err := db.Create(&session).Error; err != nil {
		t.Fatal(err)
	}
	key := strings.Repeat("A", 43)
	s := models.Server{Name: "fixture", Token: service.HashToken(key), Status: "pending"}
	if err := db.Create(&s).Error; err != nil {
		t.Fatal(err)
	}
	hub := live.New()
	defer hub.Close()
	a := New(db, config.Config{DataDir: dir, WebDir: dir, OfflineAfter: 90 * time.Second, RequireReportFreshness: true}, hub)
	router := a.Router()
	request := func(method, path, body, contentType, origin, bearer string, login bool) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", contentType)
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		if bearer != "" {
			req.Header.Set("Authorization", "Bearer "+bearer)
		}
		if login {
			req.AddCookie(&http.Cookie{Name: "cloudease_session", Value: "session-fixture"})
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}
	for _, tt := range []struct {
		name, path, body, ctype, origin string
		want                            int
	}{
		{"missing content type", "/api/servers", `{"name":"valid"}`, "", "", 415},
		{"unknown property", "/api/servers", `{"name":"valid","role":"admin"}`, "application/json", "", 400},
		{"trailing JSON", "/api/servers", `{"name":"valid"} {}`, "application/json", "", 400},
		{"oversize", "/api/servers", `{"name":"` + strings.Repeat("x", 70000) + `"}`, "application/json", "", 413},
		{"trailing oversize", "/api/servers", `{"name":"valid"}` + strings.Repeat(" ", 70000), "application/json", "", 413},
		{"CSRF", "/api/servers", `{"name":"valid"}`, "application/json", "https://evil.example", 403},
		{"invalid ID", "/api/servers/-1", `{"name":"valid"}`, "application/json", "", 400},
	} {
		t.Run(tt.name, func(t *testing.T) {
			w := request("POST", tt.path, tt.body, tt.ctype, tt.origin, "", true)
			if w.Code != tt.want {
				t.Fatalf("status %d want %d: %s", w.Code, tt.want, w.Body.String())
			}
		})
	}
	req := httptest.NewRequest("GET", "/api/public/ws", nil)
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Origin", "https://example.com")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != 403 {
		t.Fatal("cross-scheme WS accepted")
	}
	req = httptest.NewRequest("POST", "/api/servers", strings.NewReader(`{"name":"valid"}`))
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "cloudease_session", Value: "session-fixture"})
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != 403 {
		t.Fatal("cross-site fetch accepted")
	}
	body := func(at int64, nonce string) string {
		b, _ := json.Marshal(shared.Report{SentAt: at, Nonce: nonce})
		return string(b)
	}
	at := time.Now().UnixMilli()
	nonce := strings.Repeat("A", 22)
	for _, tt := range []struct {
		body string
		want int
	}{
		{body(0, ""), 400}, {body(at-600000, nonce), 400}, {body(at+600000, nonce), 400}, {body(at, strings.Repeat("!", 22)), 400}, {body(at, nonce), 200},
	} {
		w := request("POST", "/api/agent/report", tt.body, "application/json", "", key, false)
		if w.Code != tt.want {
			t.Fatalf("freshness %d want %d: %s", w.Code, tt.want, w.Body.String())
		}
	}
	if err := db.Model(&s).Update("last_seen", time.Now().Add(-10*time.Second)).Error; err != nil {
		t.Fatal(err)
	}
	w = request("POST", "/api/agent/report", body(at, nonce), "application/json", "", key, false)
	if w.Code != 409 {
		t.Fatal("replay not rejected", w.Code)
	}
	w = request("POST", "/api/agent/report", body(at+1, strings.Repeat("B", 22)), "application/json", "", key, false)
	if w.Code != 200 {
		t.Fatal("fresh report refused", w.Code, w.Body.String())
	}
	var count int64
	db.Model(&models.Metric{}).Count(&count)
	if count != 2 {
		t.Fatal("replay wrote a metric")
	}
	if validReport(shared.Report{Uptime: math.MaxUint64}) || validReport(shared.Report{NetworkTX: math.MaxUint64}) {
		t.Fatal("SQLite integer overflow accepted")
	}
	if validReport(shared.Report{CPU: math.NaN()}) {
		t.Fatal("nonfinite number accepted")
	}
	w = request("GET", "/api/public/servers/1/metrics?hours=999", "", "", "", "", false)
	if w.Code != 400 {
		t.Fatal("unbounded history")
	}
	if w.Header().Get("Cache-Control") != "no-store" || !strings.Contains(w.Header().Get("Content-Security-Policy"), "frame-ancestors 'none'") {
		t.Fatal("missing security headers")
	}
	// Credential cannot authorize another role.
	w = request("GET", "/api/servers", "", "", "", key, false)
	if w.Code != 401 {
		t.Fatal("client key became admin")
	}
	w = request("POST", "/api/agent/report", body(at+2, strings.Repeat("C", 22)), "application/json", "", "", true)
	if w.Code != 401 {
		t.Fatal("admin cookie became client key")
	}
	// A compromised old key must not poison the new key's freshness state.
	if err := db.Model(&s).Update("last_report_at", time.Now().Add(time.Minute).UnixMilli()).Error; err != nil {
		t.Fatal(err)
	}
	w = request("POST", "/api/servers/1/token", "", "application/json", "", "", true)
	if w.Code != 200 {
		t.Fatal("rotation failed", w.Code)
	}
	var rotated struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &rotated); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&s).Update("last_seen", time.Now().Add(-10*time.Second)).Error; err != nil {
		t.Fatal(err)
	}
	fresh := body(time.Now().UnixMilli(), strings.Repeat("C", 22))
	if request("POST", "/api/agent/report", fresh, "application/json", "", key, false).Code != 401 {
		t.Fatal("old key not revoked")
	}
	w = request("POST", "/api/agent/report", fresh, "application/json", "", rotated.Token, false)
	if w.Code != 200 {
		t.Fatal("old key poisoned new freshness state", w.Code, w.Body.String())
	}
	db.Delete(&user)
	w = request("GET", "/api/auth/me", "", "", "", "", true)
	if w.Code != 401 {
		t.Fatal("orphan session accepted")
	}
}
func TestRequestLimitsHaveBoundedMemory(t *testing.T) {
	l := newLimiter()
	now := time.Now()
	for i := 0; i < 60; i++ {
		if !l.allow("192.0.2.1", now) {
			t.Fatal("premature rejection")
		}
	}
	if l.allow("192.0.2.1", now) {
		t.Fatal("burst exceeded")
	}
	if !l.allow("192.0.2.1", now.Add(time.Second)) {
		t.Fatal("no refill")
	}
	for i := 0; i < 11000; i++ {
		l.allow(fmt.Sprint(i), now)
	}
	if len(l.entries) > 10000 {
		t.Fatal("unbounded limiter map")
	}
	if !l.allow("new", now.Add(6*time.Minute)) || len(l.entries) != 1 {
		t.Fatal("stale entries not purged")
	}
}
func TestCanonicalOriginBehindProxy(t *testing.T) {
	dir := t.TempDir()
	db, err := database.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	router := New(db, config.Config{WebDir: dir, PublicURL: "https://monitor.example", CookieSecure: true}, live.New()).Router()
	for _, tt := range []struct {
		path, host, origin string
		want               int
	}{
		{"/api/public/servers", "monitor.example", "", 200}, {"/api/public/servers", "wrong.example", "", 421},
		{"/api/auth/login", "monitor.example", "http://monitor.example", 403}, {"/healthz", "127.0.0.1:8080", "", 200},
	} {
		req := httptest.NewRequest("GET", tt.path, nil)
		req.Host = tt.host
		req.Header.Set("Origin", tt.origin)
		if tt.path == "/api/auth/login" {
			req.Method = "POST"
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != tt.want {
			t.Fatalf("%s: %d", tt.path, w.Code)
		}
		if w.Header().Get("Strict-Transport-Security") == "" {
			t.Fatal("missing HSTS")
		}
	}
}
func TestDownloadChecksumAndFileTypes(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	db, err := database.Open(filepath.Join(dir, "data"))
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	key := strings.Repeat("A", 43)
	if err := db.Create(&models.Server{Name: "fixture", Token: service.HashToken(key)}).Error; err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir("downloads", 0700); err != nil {
		t.Fatal(err)
	}
	data := []byte("binary fixture")
	if err := os.WriteFile("downloads/cloud-agent.exe", data, 0600); err != nil {
		t.Fatal(err)
	}
	router := New(db, config.Config{WebDir: dir}, live.New()).Router()
	call := func(path, key string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", path, nil)
		if key != "" {
			req.Header.Set("Authorization", "Bearer "+key)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}
	path := "/api/agent/downloads/cloud-agent.exe/sha256"
	if call(path, "").Code != 401 {
		t.Fatal("checksum unauthenticated")
	}
	sum := sha256.Sum256(data)
	w := call(path, key)
	if w.Code != 200 || strings.TrimSpace(w.Body.String()) != hex.EncodeToString(sum[:]) {
		t.Fatal("bad checksum", w.Code, w.Body.String())
	}
	if call("/api/agent/downloads/.env/sha256", key).Code != 404 {
		t.Fatal("checksum allowlist bypass")
	}
	if err := os.Symlink("cloud-agent.exe", "downloads/cloud-agent-linux-amd64"); err == nil {
		if call("/api/agent/downloads/cloud-agent-linux-amd64", key).Code != 404 {
			t.Fatal("symlink download accepted")
		}
	}
}
