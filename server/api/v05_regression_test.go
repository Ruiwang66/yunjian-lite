package api

import (
	"cloudease-lite/server/config"
	"cloudease-lite/server/database"
	"cloudease-lite/server/models"
	"cloudease-lite/server/service"
	live "cloudease-lite/server/websocket"
	"cloudease-lite/shared"
	"context"
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestV05HealthcheckCancelsWhenDatabaseConnectionIsBusy(t *testing.T) {
	f := newManagementFixture(t)
	sqlDB, _ := f.db.DB()
	held, err := sqlDB.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := httptest.NewRequest("GET", "/healthz", nil).WithContext(ctx)
	w := httptest.NewRecorder()
	done := make(chan struct{})
	go func() { defer close(done); f.router.ServeHTTP(w, req) }()
	cancel()
	select {
	case <-done:
		if w.Code != 503 {
			t.Fatal("busy cancelled healthcheck should be unavailable", w.Code)
		}
	case <-time.After(time.Second):
		held.Close()
		<-done
		t.Fatal("healthcheck retained a database waiter after request cancellation")
	}
}

func TestV05ForwardedHeadersAndIdentityBudgets(t *testing.T) {
	f := newManagementFixture(t)
	var w *httptest.ResponseRecorder
	for i := 0; i < 70; i++ {
		req := httptest.NewRequest("GET", "/api/public/servers", nil)
		req.Header.Set("X-Forwarded-For", fmt.Sprintf("203.0.113.%d", i+1))
		req.Header.Set("X-Real-IP", fmt.Sprintf("203.0.113.%d", i+1))
		w = httptest.NewRecorder()
		f.router.ServeHTTP(w, req)
	}
	if w.Code != 429 {
		t.Fatalf("forged forwarding headers bypassed peer budget: %d", w.Code)
	}
	if w := f.call("GET", "/api/auth/me", nil, "operator", ""); w.Code != 200 {
		t.Fatal("public flood consumed user budget", w.Code)
	}
	if w := f.call("GET", "/api/agent/check", nil, "", f.key); w.Code != 200 {
		t.Fatal("public flood consumed Agent budget", w.Code)
	}
	for i := 0; i < 70; i++ {
		w = f.call("GET", "/api/agent/check", nil, "", f.key)
	}
	if w.Code != 429 || w.Header().Get("Retry-After") == "" {
		t.Fatal("Agent identity budget missing")
	}
}

func TestV05ListLatestSamplesAndPublicProjection(t *testing.T) {
	f := newManagementFixture(t)
	now := time.Now().UTC()
	for _, cpu := range []float64{12, 34} {
		if err := f.db.Create(&models.Metric{ServerID: 1, CPU: cpu, CreatedAt: now}).Error; err != nil {
			t.Fatal(err)
		}
	}
	var queries atomic.Int32
	if err := f.db.Callback().Query().After("gorm:query").Register("v05_count_queries", func(tx *gorm.DB) {
		if !tx.DryRun {
			queries.Add(1)
		}
	}); err != nil {
		t.Fatal(err)
	}
	w := f.call("GET", "/api/public/servers", nil, "", "")
	if err := f.db.Callback().Query().Remove("v05_count_queries"); err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 || queries.Load() != 2 {
		t.Fatalf("list should need two queries: %d, %d", w.Code, queries.Load())
	}
	var rows []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["latest"].(map[string]any)["cpu"] != 34.0 {
		t.Fatal("latest tie resolution wrong", w.Body.String())
	}
	for _, key := range []string{"ip", "token", "system_json", "management_json", "management_seen_at", "last_report_nonce"} {
		if _, ok := rows[0][key]; ok {
			t.Fatal("public projection leaked", key)
		}
	}
}

type downloadDeadlineRecorder struct {
	*httptest.ResponseRecorder
	deadline time.Time
}

func (w *downloadDeadlineRecorder) SetWriteDeadline(at time.Time) error { w.deadline = at; return nil }
func TestV05DownloadsHaveSeparateBoundedWriteDeadline(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.Mkdir("downloads", 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("downloads", "cloud-agent.exe"), []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	f := newManagementFixture(t)
	req := httptest.NewRequest("GET", "/api/downloads/cloud-agent.exe", nil)
	req.AddCookie(f.cookies["viewer"])
	w := &downloadDeadlineRecorder{ResponseRecorder: httptest.NewRecorder()}
	f.router.ServeHTTP(w, req)
	if w.Code != http.StatusOK || w.Body.String() != "fixture" || time.Until(w.deadline) < 4*time.Minute || time.Until(w.deadline) > 5*time.Minute {
		t.Fatal("download deadline or content wrong", w.Code, w.deadline)
	}
	hub := live.New()
	defer hub.Close()
	a := New(f.db, config.Config{WebDir: t.TempDir()}, hub)
	for i := 0; i < cap(a.downloadSlots); i++ {
		a.downloadSlots <- struct{}{}
	}
	blocked := httptest.NewRecorder()
	a.Router().ServeHTTP(blocked, req.Clone(req.Context()))
	if blocked.Code != 503 || blocked.Header().Get("Retry-After") == "" {
		t.Fatal("unbounded concurrent downloads", blocked.Code)
	}
}

func TestV05FullLoginMapStillAdmitsExistingBelowLimit(t *testing.T) {
	a := &API{attempts: map[string]attempt{}}
	now := time.Now()
	for i := 0; i < 10000; i++ {
		a.attempts[fmt.Sprint(i)] = attempt{Count: 1, Until: now.Add(time.Minute)}
	}
	if !a.allowLogin("0") || a.allowLogin("new-key") {
		t.Fatal("login map capacity mishandled")
	}
}

func TestV05PasswordResetDuringLoginCannotMintStaleSession(t *testing.T) {
	f := newManagementFixture(t)
	read, resume := make(chan struct{}), make(chan struct{})
	var captured atomic.Bool
	if err := f.db.Callback().Query().After("gorm:query").Register("v05_pause_login", func(tx *gorm.DB) {
		if tx.Statement.Table == "users" && captured.CompareAndSwap(false, true) {
			close(read)
			<-resume
		}
	}); err != nil {
		t.Fatal(err)
	}
	defer f.db.Callback().Query().Remove("v05_pause_login")
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		done <- f.call("POST", "/api/auth/login", map[string]any{"username": "operator", "password": "secure-v02-password"}, "", "")
	}()
	select {
	case <-read:
	case <-time.After(3 * time.Second):
		close(resume)
		t.Fatal("login did not reach password read")
	}
	err := f.db.Model(&models.User{}).Where("username = ?", "operator").Update("password", "changed-during-login").Error
	close(resume)
	if err != nil {
		t.Fatal(err)
	}
	w := <-done
	if w.Code != 401 || len(w.Result().Cookies()) != 0 {
		t.Fatalf("old password created a post-reset session: %d", w.Code)
	}
}

func TestV05RevokedSessionCannotCommitTaskResult(t *testing.T) {
	for _, reason := range []string{"logout", "expiry", "demotion"} {
		t.Run(reason, func(t *testing.T) {
			f := newManagementFixture(t)
			f.task("GET", "/api/docker/container?server_id=1", nil, "operator")
			task := f.lease()
			var user models.User
			if err := f.db.Where("username = ?", "operator").First(&user).Error; err != nil {
				t.Fatal(err)
			}
			switch reason {
			case "logout":
				if w := f.call("POST", "/api/auth/logout", nil, "operator", ""); w.Code != 200 {
					t.Fatal(w.Code)
				}
			case "expiry":
				if err := f.db.Model(&models.Session{}).Where("user_id = ?", user.ID).Update("expires_at", time.Now().UTC().Add(-time.Second)).Error; err != nil {
					t.Fatal(err)
				}
			case "demotion":
				if err := f.db.Model(&user).Update("role", "invalid").Error; err != nil {
					t.Fatal(err)
				}
			}
			result := shared.ManagementResult{ID: task.ID, Lease: task.Lease, Success: true, Data: json.RawMessage(`[{"id":"abcdef123456","name":"must-not-cache"}]`)}
			w := f.call("POST", "/api/agent/operations/result", result, "", f.key)
			if w.Code != 409 {
				t.Fatalf("revoked authorization committed result: %d %s", w.Code, w.Body.String())
			}
			var count int64
			if err := f.db.Model(&models.DockerContainer{}).Count(&count).Error; err != nil || count != 0 {
				t.Fatalf("cache changed: %d %v", count, err)
			}
		})
	}
}

func TestV05AgentsBehindNATHaveIndependentBudgets(t *testing.T) {
	f := newManagementFixture(t)
	for i := 0; i < 80; i++ {
		key, err := service.RandomToken()
		if err != nil {
			t.Fatal(err)
		}
		if err := f.db.Create(&models.Server{Name: "NAT fixture", Token: service.HashToken(key)}).Error; err != nil {
			t.Fatal(err)
		}
		if w := f.call("GET", "/api/agent/check", nil, "", key); w.Code != 200 {
			t.Fatalf("NAT node %d rejected: %d", i, w.Code)
		}
	}
}

func TestV05AgentCannotPoisonWebsiteCacheTime(t *testing.T) {
	f := newManagementFixture(t)
	var s models.Server
	if err := f.db.First(&s, 1).Error; err != nil {
		t.Fatal(err)
	}
	var info shared.ManagementInfo
	if err := json.Unmarshal([]byte(s.ManagementJSON), &info); err != nil {
		t.Fatal(err)
	}
	spec := shared.WebsiteSpec{Name: "fixture", Domain: "blog.example.com", RootPath: "/srv/blog", NginxConfig: "blog.conf"}
	info.Websites = &shared.WebsiteInventory{Enabled: true, Sites: []shared.WebsiteSpec{spec}, Checks: []shared.WebsiteCheck{{Domain: spec.Domain, CheckedAt: time.Now().Add(365 * 24 * time.Hour).UnixMilli()}}}
	if err := f.db.Create(&models.Website{ServerID: s.ID, Name: spec.Name, Domain: spec.Domain, RootPath: spec.RootPath, NginxConfig: spec.NginxConfig}).Error; err != nil {
		t.Fatal(err)
	}
	r := shared.Report{OS: "linux", SentAt: time.Now().UnixMilli() + 1, Nonce: strings.Repeat("F", 22), Management: &info}
	if err := f.db.Model(&s).Update("last_seen", time.Now().UTC().Add(-time.Minute)).Error; err != nil {
		t.Fatal(err)
	}
	if w := f.call("POST", "/api/agent/report", r, "", f.key); w.Code != 400 {
		t.Fatalf("future check accepted: %d %s", w.Code, w.Body.String())
	}
}

func BenchmarkV05ServerList(b *testing.B) {
	_, a, cleanup := benchmarkListAPI(b)
	defer cleanup()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		a.servers(c)
		if w.Code != 200 {
			b.Fatal(w.Code)
		}
	}
}

func benchmarkListAPI(b *testing.B) (*gorm.DB, *API, func()) {
	b.Helper()
	gin.SetMode(gin.ReleaseMode)
	db, err := database.Open(b.TempDir())
	if err != nil {
		b.Fatal(err)
	}
	conn, _ := db.DB()
	hub := live.New()
	a := New(db, config.Config{OfflineAfter: 90 * time.Second}, hub)
	now := time.Now().UTC()
	for i := 0; i < 32; i++ {
		key, _ := service.RandomToken()
		s := models.Server{Name: "benchmark host", Token: service.HashToken(key), LastSeen: &now}
		if err := db.Create(&s).Error; err != nil {
			b.Fatal(err)
		}
		rows := make([]models.Metric, 900)
		for j := range rows {
			rows[j] = models.Metric{ServerID: s.ID, CPU: float64(j % 100), CreatedAt: now.Add(time.Duration(j-899) * 4 * time.Second)}
		}
		if err := db.CreateInBatches(rows, 200).Error; err != nil {
			b.Fatal(err)
		}
	}
	return db, a, func() { hub.Close(); conn.Close() }
}
