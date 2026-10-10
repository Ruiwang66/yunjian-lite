package api

import (
	"cloudease-lite/server/config"
	"cloudease-lite/server/database"
	"cloudease-lite/server/models"
	"cloudease-lite/server/service"
	live "cloudease-lite/server/websocket"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestUninstallPreparationAndRevocation(t *testing.T) {
	dir := t.TempDir()
	db, err := database.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	key := strings.Repeat("A", 43)
	server := models.Server{Name: "Client removal fixture", Token: service.HashToken(key), Status: "pending"}
	if err := db.Create(&server).Error; err != nil {
		t.Fatal(err)
	}
	user := models.User{Username: "admin", Password: "fixture-unused"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	session := models.Session{UserID: user.ID, TokenHash: service.HashToken("admin-fixture"), ExpiresAt: time.Now().Add(time.Hour)}
	if err := db.Create(&session).Error; err != nil {
		t.Fatal(err)
	}
	router := New(db, config.Config{WebDir: dir}, live.New()).Router()
	call := func(method, path string, admin bool, bearer string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, nil)
		if admin {
			req.AddCookie(&http.Cookie{Name: "cloudease_session", Value: "admin-fixture"})
		}
		if bearer != "" {
			req.Header.Set("Authorization", "Bearer "+bearer)
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}
	path := "/api/servers/1/uninstall"
	for _, bearer := range []string{"", key} {
		if w := call("GET", path, false, bearer); w.Code != 401 {
			t.Fatalf("uninstall metadata must require an admin session: %d", w.Code)
		}
	}
	w := call("GET", path, true, "")
	var metadata struct {
		Fingerprint string `json:"key_fingerprint"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &metadata); err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 || metadata.Fingerprint != service.HashToken(key) || strings.Contains(w.Body.String(), key) || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("uninstall preparation must return only the matching fingerprint without caching")
	}
	if w := call("GET", "/api/servers/999/uninstall", true, ""); w.Code != 404 {
		t.Fatal("missing record must be rejected")
	}
	if w := call("GET", "/api/agent/check", false, key); w.Code != 200 {
		t.Fatal("existing key invalid before deletion")
	}
	if w := call("DELETE", "/api/servers/1", true, ""); w.Code != 200 {
		t.Fatal("server deletion failed")
	}
	if w := call("GET", "/api/agent/check", false, key); w.Code != 401 {
		t.Fatal("deleted connection key still authenticates")
	}
	if w := call("GET", path, true, ""); w.Code != 404 {
		t.Fatal("deleted metadata still exists")
	}
	for path, marker := range map[string]string{
		"/uninstall-client.sh":  "--key-fingerprint",
		"/uninstall-client.ps1": "$KeyFingerprint",
	} {
		w := call("GET", path, false, "")
		if w.Code != 200 || !strings.Contains(w.Body.String(), marker) || w.Header().Get("Cache-Control") != "no-store" || strings.Contains(w.Body.String(), key) {
			t.Fatalf("uninstall script must remain available after revocation without any embedded key: %s", path)
		}
	}
}
