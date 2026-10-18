package api

import (
	"cloudease-lite/server/config"
	"cloudease-lite/server/database"
	"cloudease-lite/server/models"
	"cloudease-lite/server/service"
	live "cloudease-lite/server/websocket"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAgentInstallerDownloadAuthorization(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	db, err := database.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	s := models.Server{Name: "Installation fixture", Token: service.HashToken("fixture-token"), Status: "pending"}
	if err := db.Create(&s).Error; err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir("downloads", 0755); err != nil {
		t.Fatal(err)
	}
	binary := []byte("fixture-agent-binary")
	if err := os.WriteFile(filepath.Join("downloads", "cloud-agent-linux-amd64"), binary, 0755); err != nil {
		t.Fatal(err)
	}
	router := New(db, config.Config{WebDir: dir}, live.New()).Router()
	call := func(path, auth string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", path, nil)
		if auth != "" {
			req.Header.Set("Authorization", auth)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}
	for _, path := range []string{"/api/agent/check", "/api/agent/downloads/cloud-agent-linux-amd64"} {
		for _, auth := range []string{"", "Bearer wrong-token"} {
			if w := call(path, auth); w.Code != 401 {
				t.Fatalf("unauthorized %s: %d", path, w.Code)
			}
		}
	}
	if w := call("/api/agent/check?token=fixture-token", ""); w.Code != 401 {
		t.Fatal("query string must not authenticate")
	}
	if w := call("/api/agent/check", "Bearer fixture-token"); w.Code != 200 {
		t.Fatalf("valid check: %d", w.Code)
	}
	if w := call("/api/agent/downloads/cloud-agent-linux-amd64", "Bearer fixture-token"); w.Code != 200 || w.Body.String() != string(binary) {
		t.Fatal("valid token could not download Agent")
	}
	if w := call("/api/agent/downloads/.env", "Bearer fixture-token"); w.Code != 404 {
		t.Fatal("download allowlist bypass")
	}
	if w := call("/api/downloads/cloud-agent-linux-amd64", "Bearer fixture-token"); w.Code != 401 {
		t.Fatal("Agent token must not grant administrator access")
	}
	for path, marker := range map[string]string{"/install-client.sh": "--container-monitor", "/install-client.ps1": "Register-ScheduledTask", "/install-agent.sh": "--container-monitor", "/install-agent.ps1": "Register-ScheduledTask"} {
		w := call(path, "")
		if w.Code != 200 || !strings.Contains(w.Body.String(), marker) || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("missing installer: %s", path)
		}
	}
	var after models.Server
	db.First(&after, s.ID)
	var metrics, logs int64
	db.Model(&models.Metric{}).Count(&metrics)
	db.Model(&models.SystemLog{}).Count(&logs)
	if after.Status != "pending" || after.LastSeen != nil || metrics != 0 || logs != 0 {
		t.Fatal("installation check must be read-only")
	}
}
