package api

import (
	"bytes"
	"cloudease-lite/server/config"
	"cloudease-lite/server/database"
	"cloudease-lite/server/models"
	"cloudease-lite/server/service"
	live "cloudease-lite/server/websocket"
	"cloudease-lite/shared"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type managementFixture struct {
	t       *testing.T
	db      *gorm.DB
	router  *gin.Engine
	cookies map[string]*http.Cookie
	key     string
}

func newManagementFixture(t *testing.T) *managementFixture {
	t.Helper()
	dir := t.TempDir()
	db, err := database.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	conn, _ := db.DB()
	t.Cleanup(func() { conn.Close() })
	hub := live.New()
	t.Cleanup(hub.Close)
	a := New(db, config.Config{DataDir: dir, WebDir: dir, SessionTTL: time.Hour, OfflineAfter: 90 * time.Second, RequireReportFreshness: true}, hub)
	f := &managementFixture{t: t, db: db, router: a.Router(), cookies: map[string]*http.Cookie{}, key: strings.Repeat("K", 43)}
	hash, _ := bcrypt.GenerateFromPassword([]byte("secure-v02-password"), bcrypt.MinCost)
	for _, role := range []string{"admin", "operator", "viewer"} {
		u := models.User{Username: role, Role: role, Password: string(hash)}
		if db.Create(&u).Error != nil {
			t.Fatal("create user")
		}
		w := f.call("POST", "/api/auth/login", map[string]any{"username": role, "password": "secure-v02-password"}, "", "")
		if w.Code != 200 {
			t.Fatalf("%s login failed: %d %s", role, w.Code, w.Body.String())
		}
		var data struct{ User models.User }
		json.Unmarshal(w.Body.Bytes(), &data)
		if data.User.Role != role {
			t.Fatal("role not returned")
		}
		f.cookies[role] = w.Result().Cookies()[0]
	}
	s := models.Server{Name: "v02-host", Token: service.HashToken(f.key), Status: "pending"}
	if db.Create(&s).Error != nil {
		t.Fatal("create server")
	}
	report := shared.Report{SentAt: time.Now().UnixMilli(), Nonce: strings.Repeat("A", 22), OS: "linux", System: &shared.SystemInfo{CPUModel: "Test CPU", PrivateIPs: []string{"10.0.0.2"}}, Management: &shared.ManagementInfo{Version: shared.Version, Enabled: true, Privileged: true, DockerWrite: true, ServiceWrite: true, FileWrite: true, FileRoots: []string{"/opt/cloud-ease"}, Docker: shared.DockerInfo{Available: true, Version: "27", Containers: 2, Images: 3}}}
	w := f.call("POST", "/api/agent/report", report, "", f.key)
	if w.Code != 200 {
		t.Fatalf("enhanced report: %d %s", w.Code, w.Body.String())
	}
	return f
}
func (f *managementFixture) call(method, path string, body any, role, key string) *httptest.ResponseRecorder {
	f.t.Helper()
	var raw []byte
	if body != nil {
		raw, _ = json.Marshal(body)
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	if cookie := f.cookies[role]; cookie != nil {
		req.AddCookie(cookie)
	}
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	w := httptest.NewRecorder()
	f.router.ServeHTTP(w, req)
	return w
}
func (f *managementFixture) task(method, path string, body any, role string) string {
	f.t.Helper()
	w := f.call(method, path, body, role, "")
	if w.Code != 202 {
		f.t.Fatalf("queue operation: %d %s", w.Code, w.Body.String())
	}
	var result map[string]any
	json.Unmarshal(w.Body.Bytes(), &result)
	return result["id"].(string)
}
func (f *managementFixture) lease() shared.ManagementTask {
	f.t.Helper()
	w := f.call("GET", "/api/agent/operations/next", nil, "", f.key)
	if w.Code != 200 {
		f.t.Fatalf("lease: %d %s", w.Code, w.Body.String())
	}
	var task shared.ManagementTask
	json.Unmarshal(w.Body.Bytes(), &task)
	return task
}
func TestV02RolePermissionsAndPrivateData(t *testing.T) {
	f := newManagementFixture(t)
	for _, path := range []string{"/api/docker/info?server_id=1", "/api/docker/container?server_id=1", "/api/service/list?server_id=1", "/api/files/list?server_id=1", "/api/system/info?server_id=1", "/api/operation-logs", "/api/users"} {
		if w := f.call("GET", path, nil, "", ""); w.Code != 401 {
			t.Fatalf("unauthenticated route %s: %d", path, w.Code)
		}
	}
	for _, role := range []string{"viewer", "operator"} {
		if w := f.call("GET", "/api/users", nil, role, ""); w.Code != 403 {
			t.Fatalf("users accessible to %s", role)
		}
	}
	body := map[string]any{"server_id": 1, "name": "nginx", "action": "restart"}
	if w := f.call("POST", "/api/service/action", body, "viewer", ""); w.Code != 403 {
		t.Fatal("viewer mutation allowed")
	}
	if w := f.call("POST", "/api/commands/execute", map[string]any{"server_id": 1, "name": "disk"}, "viewer", ""); w.Code != 403 {
		t.Fatal("viewer command allowed")
	}
	if w := f.call("POST", "/api/servers", map[string]any{"name": "new"}, "viewer", ""); w.Code != 403 {
		t.Fatal("viewer registration allowed")
	}
	if w := f.call("POST", "/api/commands/execute", map[string]any{"server_id": 1, "name": "rm -rf /"}, "operator", ""); w.Code != 400 {
		t.Fatal("arbitrary command accepted")
	}
	if w := f.call("POST", "/api/docker/container/start", map[string]any{"server_id": 1, "id": "abc;id"}, "operator", ""); w.Code != 400 {
		t.Fatal("container injection accepted")
	}
	if w := f.call("GET", "/api/files/read?server_id=1&path=../secret", nil, "viewer", ""); w.Code != 400 {
		t.Fatal("path traversal accepted")
	}
	if w := f.call("GET", "/api/system/info?server_id=1", nil, "viewer", ""); w.Code != 200 || !strings.Contains(w.Body.String(), "Test CPU") {
		t.Fatal("viewer cannot read system info")
	}
	if w := f.call("GET", "/api/public/servers/1", nil, "", ""); w.Code != 200 || strings.Contains(w.Body.String(), "Test CPU") || strings.Contains(w.Body.String(), "10.0.0.2") || strings.Contains(w.Body.String(), "management") {
		t.Fatal("public metadata leaked")
	}
	if w := f.call("GET", "/api/agent/operations/next", nil, "admin", ""); w.Code != 401 {
		t.Fatal("browser session substituted for agent key")
	}
}
func TestV02TaskLeaseResultsCacheAndAudit(t *testing.T) {
	f := newManagementFixture(t)
	id := f.task("GET", "/api/docker/container?server_id=1", nil, "viewer")
	task := f.lease()
	if task.ID != id || task.Request.Action != "docker.list" {
		t.Fatalf("wrong task: %+v", task)
	}
	if w := f.call("GET", "/api/agent/operations/next", nil, "", f.key); w.Code != 204 {
		t.Fatal("running task leased twice")
	}
	data := json.RawMessage(`[{"id":"aaaaaaaaaaaa","name":"web","state":"running","ports":"80/tcp","created_at":"2026-10-11"}]`)
	result := shared.ManagementResult{ID: id, Lease: task.Lease, Success: true, Data: data}
	bad := result
	bad.Lease = strings.Repeat("Z", 43)
	if w := f.call("POST", "/api/agent/operations/result", bad, "", f.key); w.Code != 409 {
		t.Fatal("wrong lease accepted")
	}
	if w := f.call("POST", "/api/agent/operations/result", result, "", f.key); w.Code != 200 {
		t.Fatalf("result: %d %s", w.Code, w.Body.String())
	}
	if w := f.call("POST", "/api/agent/operations/result", result, "", f.key); w.Code != 409 {
		t.Fatal("result replay accepted")
	}
	if w := f.call("GET", "/api/operations/"+id, nil, "viewer", ""); w.Code != 200 || !strings.Contains(w.Body.String(), "running") {
		t.Fatal("result unavailable")
	}
	if w := f.call("GET", "/api/operations/"+id, nil, "operator", ""); w.Code != 403 {
		t.Fatal("another user's result accessible")
	}
	var count int64
	f.db.Model(&models.DockerContainer{}).Where("server_id = ?", 1).Count(&count)
	if count != 1 {
		t.Fatal("container cache missing")
	}
	id = f.task("POST", "/api/service/action", map[string]any{"server_id": 1, "name": "nginx", "action": "restart"}, "operator")
	task = f.lease()
	result = shared.ManagementResult{ID: id, Lease: task.Lease, Success: true, Data: json.RawMessage(`{"output":"password: vault-value\nAuthorization: Bearer topsecret"}`)}
	if w := f.call("POST", "/api/agent/operations/result", result, "", f.key); w.Code != 200 {
		t.Fatalf("operation result: %s", w.Body.String())
	}
	w := f.call("GET", "/api/operations/"+id, nil, "operator", "")
	if strings.Contains(w.Body.String(), "vault-value") || strings.Contains(w.Body.String(), "topsecret") {
		t.Fatal("credentials retained in operation output")
	}
	var entry models.OperationLog
	f.db.Where("action = ?", "service.restart").First(&entry)
	if entry.User != "operator" || entry.Status != "success" || entry.IP == "" {
		t.Fatalf("audit missing: %+v", entry)
	}
	id = f.task("GET", "/api/service/list?server_id=1", nil, "viewer")
	task = f.lease()
	result = shared.ManagementResult{ID: id, Lease: task.Lease, Success: true, Data: json.RawMessage(`[{"name":"nginx.service","state":"active","enabled":"enabled","allowed":true}]`)}
	if w := f.call("POST", "/api/agent/operations/result", result, "", f.key); w.Code != 200 {
		t.Fatal("service snapshot rejected")
	}
	f.db.Model(&models.Service{}).Count(&count)
	if count != 1 {
		t.Fatal("service cache missing")
	}
	id = f.task("GET", "/api/files/list?server_id=1&path=.", nil, "viewer")
	task = f.lease()
	result = shared.ManagementResult{ID: id, Lease: task.Lease, Success: true, Data: json.RawMessage(`[{"name":"readme.txt","directory":false,"size":12,"modified_at":1}]`)}
	if w := f.call("POST", "/api/agent/operations/result", result, "", f.key); w.Code != 200 {
		t.Fatal("file metadata rejected")
	}
	f.db.Model(&models.File{}).Count(&count)
	if count != 1 {
		t.Fatal("file cache missing")
	}
}
func TestV02RevocationExpiryAndNoRedispatch(t *testing.T) {
	f := newManagementFixture(t)
	id := f.task("POST", "/api/commands/execute", map[string]any{"server_id": 1, "name": "disk"}, "operator")
	task := f.lease()
	f.db.Model(&models.ManagementTask{}).Where("id = ?", id).Update("expires_at", time.Now().UTC().Add(-time.Second))
	if err := service.ExpireOperations(f.db, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	result := shared.ManagementResult{ID: id, Lease: task.Lease, Success: true, Data: json.RawMessage(`{"output":"disk"}`)}
	if w := f.call("POST", "/api/agent/operations/result", result, "", f.key); w.Code != 409 {
		t.Fatal("expired operation result accepted")
	}
	if w := f.call("GET", "/api/agent/operations/next", nil, "", f.key); w.Code != 204 {
		t.Fatal("expired operation redispatched")
	}
	id = f.task("POST", "/api/commands/execute", map[string]any{"server_id": 1, "name": "memory"}, "operator")
	if w := f.call("POST", "/api/auth/logout", nil, "operator", ""); w.Code != 200 {
		t.Fatal("logout failed")
	}
	if w := f.call("GET", "/api/agent/operations/next", nil, "", f.key); w.Code != 204 {
		t.Fatal("revoked session operation dispatched")
	}
	var cancelled models.ManagementTask
	f.db.First(&cancelled, "id = ?", id)
	if cancelled.Status != "cancelled" || cancelled.RequestJSON != "" {
		t.Fatal("revoked task not cancelled")
	}
	id = f.task("GET", "/api/service/list?server_id=1", nil, "viewer")
	f.db.Model(&models.Server{}).Where("id = ?", 1).Update("token", service.HashToken(strings.Repeat("N", 43)))
	if w := f.call("GET", "/api/agent/operations/next", nil, "", f.key); w.Code != 401 {
		t.Fatal("rotated key accepted")
	}
	f.key = strings.Repeat("N", 43)
	if w := f.call("GET", "/api/agent/operations/next", nil, "", f.key); w.Code != 204 {
		t.Fatal("old-key task dispatched after rotation")
	}
}
func TestV02UserAdministrationAndLastAdminProtection(t *testing.T) {
	f := newManagementFixture(t)
	if w := f.call("POST", "/api/users", map[string]any{"username": "read-only", "role": "viewer", "password": "another-secure-password"}, "admin", ""); w.Code != 201 {
		t.Fatalf("create user: %s", w.Body.String())
	}
	if w := f.call("PATCH", "/api/users/1", map[string]any{"role": "viewer"}, "admin", ""); w.Code != 409 {
		t.Fatal("last admin downgraded")
	}
	if w := f.call("DELETE", "/api/users/1", nil, "admin", ""); w.Code != 409 {
		t.Fatal("current admin deleted")
	}
	if w := f.call("PATCH", "/api/users/2", map[string]any{"role": "viewer"}, "admin", ""); w.Code != 200 {
		t.Fatal("role update failed")
	}
	if w := f.call("GET", "/api/servers", nil, "operator", ""); w.Code != 401 {
		t.Fatal("role change did not revoke sessions")
	}
	if w := f.call("GET", "/api/users", nil, "admin", ""); w.Code != 200 || strings.Contains(w.Body.String(), "$2a$") {
		t.Fatal("users API leaked password hashes")
	}
}

func TestV02EscapedSystemdNamesDoNotBreakMonitoring(t *testing.T) {
	report := shared.Report{Management: &shared.ManagementInfo{Services: []shared.Service{{Name: `systemd-cryptsetup@luks\x2d123.service`, State: "active", Enabled: "enabled", Allowed: false}}}}
	if !validExtensions(report) {
		t.Fatal("legitimate escaped unit rejected entire metrics report")
	}
	f := newManagementFixture(t)
	id := f.task("GET", "/api/service/list?server_id=1", nil, "viewer")
	task := f.lease()
	data, _ := json.Marshal(report.Management.Services)
	result := shared.ManagementResult{ID: id, Lease: task.Lease, Success: true, Data: data}
	if w := f.call("POST", "/api/agent/operations/result", result, "", f.key); w.Code != 200 {
		t.Fatalf("escaped service snapshot: %d %s", w.Code, w.Body.String())
	}
	if w := f.call("POST", "/api/service/action", map[string]any{"server_id": 1, "name": report.Management.Services[0].Name, "action": "restart"}, "operator", ""); w.Code != 400 {
		t.Fatal("escaped target bypassed operation whitelist")
	}
}
func TestV02ViewerLoginCannotResetAdminFailures(t *testing.T) {
	f := newManagementFixture(t)
	bad := map[string]any{"username": "admin", "password": "wrong-password"}
	for i := 0; i < 9; i++ {
		if w := f.call("POST", "/api/auth/login", bad, "", ""); w.Code != 401 {
			t.Fatalf("early limit: %d", w.Code)
		}
	}
	if w := f.call("POST", "/api/auth/login", map[string]any{"username": "viewer", "password": "secure-v02-password"}, "", ""); w.Code != 200 {
		t.Fatal("valid viewer login denied")
	}
	if w := f.call("POST", "/api/auth/login", bad, "", ""); w.Code != 401 {
		t.Fatal("tenth failed attempt not evaluated")
	}
	if w := f.call("POST", "/api/auth/login", map[string]any{"username": "admin", "password": "secure-v02-password"}, "", ""); w.Code != 429 {
		t.Fatal("viewer success cleared admin brute-force protection")
	}
}
