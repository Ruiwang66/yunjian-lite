// Local-only UI fixture: real API, roles, SQLite and async task flow; simulated
// Linux operations. Never included in the production server or Docker build.
package main

import (
	"archive/zip"
	"bytes"
	"cloudease-lite/server/api"
	"cloudease-lite/server/config"
	"cloudease-lite/server/database"
	"cloudease-lite/server/models"
	"cloudease-lite/server/service"
	live "cloudease-lite/server/websocket"
	"cloudease-lite/shared"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/glebarez/sqlite"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

func main() {
	root := flag.String("root", ".runtime/v03-preview", "Isolated local fixture directory")
	web := flag.String("web", "web/dist", "Built frontend path")
	flag.Parse()
	abs, _ := filepath.Abs(*root)
	os.MkdirAll(abs, 0700)
	db, err := database.Open(filepath.Join(abs, "data"))
	if err != nil {
		log.Fatal(err)
	}
	conn, _ := db.DB()
	defer conn.Close()
	password := "CloudEase03!Test"
	hash, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	for _, role := range []string{"admin", "operator", "viewer"} {
		var count int64
		db.Model(&models.User{}).Where("username = ?", role).Count(&count)
		if count == 0 {
			db.Create(&models.User{Username: role, Role: role, Password: string(hash)})
		}
	}
	key, _ := service.RandomToken()
	var server models.Server
	db.Where("name = ?", "本地演示 · Linux 操作模拟").First(&server)
	server.Name = "本地演示 · Linux 操作模拟"
	server.Token = service.HashToken(key)
	server.Status = "online"
	if server.CountryCode == "" {
		server.CountryCode = "CN"
	} // Explicit local demo location only.
	db.Save(&server)
	specs := []shared.WebsiteSpec{{Name: "我的博客", Domain: "blog.example.com", RootPath: "/srv/demo/blog", NginxConfig: "blog.conf"}, {Name: "应用服务", Domain: "app.example.com", RootPath: "/srv/demo/app", NginxConfig: "app.conf"}, {Name: "新授权站点", Domain: "shop.example.com", RootPath: "/srv/demo/shop", NginxConfig: "shop.conf"}}
	var count int64
	db.Model(&models.Website{}).Where("server_id = ?", server.ID).Count(&count)
	if count == 0 {
		for _, s := range specs[:2] {
			db.Create(&models.Website{Name: s.Name, Domain: s.Domain, RootPath: s.RootPath, NginxConfig: s.NginxConfig, ServerID: server.ID, PHPVersion: "PHP 8.2.18", Status: "未检测"})
		}
	}
	hub := live.New()
	defer hub.Close()
	cfg := config.Config{Listen: "127.0.0.1:5053", DataDir: filepath.Join(abs, "data"), WebDir: *web, SessionTTL: 24 * time.Hour, OfflineAfter: 90 * time.Second, RetentionDays: 30, RequireReportFreshness: true}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go service.Monitor(ctx, db, cfg.OfflineAfter, 30, hub.Broadcast)
	serverHTTP := &http.Server{Addr: cfg.Listen, Handler: api.New(db, cfg, hub).Router(), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if e := serverHTTP.ListenAndServe(); e != nil && e != http.ErrServerClosed {
			log.Fatal(e)
		}
	}()
	origin := "http://" + cfg.Listen
	client := &http.Client{Timeout: 5 * time.Second}
	enabled := map[string]bool{"blog.example.com": true, "app.example.com": true, "shop.example.com": false}
	call := func(method, path string, body any) (int, []byte) {
		raw, _ := json.Marshal(body)
		req, _ := http.NewRequestWithContext(ctx, method, origin+path, bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+key)
		response, e := client.Do(req)
		if e != nil {
			return 0, nil
		}
		defer response.Body.Close()
		data, _ := io.ReadAll(io.LimitReader(response.Body, 65536))
		return response.StatusCode, data
	}
	check := func(domain string) shared.WebsiteCheck {
		now := time.Now()
		days := 72
		if strings.HasPrefix(domain, "app.") {
			days = 6
		}
		ssl := shared.TLSInfo{Status: "有效", Issuer: "Let's Encrypt（演示）", NotBefore: now.Add(-18 * 24 * time.Hour).UnixMilli(), NotAfter: now.Add(time.Duration(days) * 24 * time.Hour).UnixMilli(), DaysLeft: days, Version: "TLS 1.3", Chain: []string{"CN=" + domain, "CN=演示中间证书"}}
		if days <= 7 {
			ssl.Warning = "高危：证书将在 7 天内过期"
		}
		return shared.WebsiteCheck{Domain: domain, Status: "正常", A: []string{"203.0.113.10"}, AAAA: []string{}, HTTPStatus: 200, ResponseMS: 220, Accessible: true, TLS: ssl, Warnings: []string{}, CheckedAt: now.UnixMilli()}
	}
	report := func() {
		checks := []shared.WebsiteCheck{}
		for _, s := range specs {
			c := check(s.Domain)
			if !enabled[s.Domain] {
				c.Status = "异常"
				c.Accessible = false
				c.HTTPStatus = 503
				c.Error = "本地演示站点已禁用"
			}
			checks = append(checks, c)
		}
		r := shared.Report{OS: "linux", Platform: "ubuntu", PlatformVersion: "24.04（模拟）", Arch: "amd64", CPUCores: 2, CPU: 5, Memory: 32, Disk: 25, MemoryTotal: 2147483648, MemoryUsed: 687194767, DiskTotal: 42949672960, DiskUsed: 10737418240, SentAt: time.Now().UnixMilli(), Nonce: strings.Repeat("A", 22), Management: &shared.ManagementInfo{Version: shared.Version, Enabled: true, Privileged: true, FileRoots: []string{"/opt/cloud-ease"}, Websites: &shared.WebsiteInventory{Demo: true, Enabled: true, Sites: specs, Checks: checks, Nginx: shared.NginxInfo{Available: true, Version: "nginx/1.24.0（演示）", State: "active"}, PHP: shared.PHPInfo{Available: true, Version: "PHP 8.2.18（演示）", FPMState: "php8.2-fpm.service: active", Extensions: []string{"curl", "gd", "mysqli", "pdo", "openssl"}, Warnings: []string{}}}}}
		nonce, _ := service.RandomToken()
		r.Nonce = nonce[:22]
		call("POST", "/api/agent/report", r)
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	nextReport := time.Now()
	log.Printf("Local v0.3 preview: %s (isolated fixture, Linux actions simulated)", origin)
	for {
		select {
		case <-ctx.Done():
			serverHTTP.Shutdown(context.Background())
			return
		case <-ticker.C:
		}
		if time.Now().After(nextReport) {
			report()
			nextReport = time.Now().Add(4 * time.Second)
		}
		code, raw := call("GET", "/api/agent/operations/next", nil)
		if code != 200 {
			continue
		}
		var task shared.ManagementTask
		if json.Unmarshal(raw, &task) != nil {
			continue
		}
		var value any
		var operationErr error
		domain := task.Request.Target
		switch task.Request.Action {
		case "website.check", "ssl.check", "domain.check":
			value = check(domain)
		case "nginx.read":
			value = shared.SiteResult{Domain: domain, Enabled: enabled[domain], Config: "# 本地演示配置\nserver {\n  listen 80;\n  server_name " + domain + ";\n  root " + task.Request.Website.RootPath + ";\n  location ~ /\\.(env|git) { deny all; }\n}", Warnings: []string{}}
		case "nginx.enable", "nginx.disable":
			enabled[domain] = task.Request.Action == "nginx.enable"
			value = shared.SiteResult{Domain: domain, Enabled: enabled[domain], Backup: "nginx-local-demo.conf", Warnings: []string{}}
		case "nginx.sites":
			rows := []shared.SiteResult{}
			for _, s := range specs {
				rows = append(rows, shared.SiteResult{Domain: s.Domain, Enabled: enabled[s.Domain]})
			}
			value = rows
		case "php.inspect":
			value = shared.PHPInfo{Available: true, Version: "PHP 8.2.18（演示）", FPMState: "active（模拟）", Extensions: []string{"curl", "gd", "mysqli", "pdo", "openssl"}, Warnings: []string{}}
		case "backup.files", "backup.database":
			kind := strings.TrimPrefix(task.Request.Action, "backup.")
			dir := filepath.Join(abs, "backups")
			os.MkdirAll(dir, 0700)
			name := fmt.Sprintf("%s-%s-%d", domain, kind, time.Now().UnixMilli())
			dest := filepath.Join(dir, name)
			if kind == "files" {
				name += ".zip"
				dest += ".zip"
				file, e := os.Create(dest)
				operationErr = e
				if e == nil {
					z := zip.NewWriter(file)
					entry, _ := z.Create("index.html")
					entry.Write([]byte("CloudEase local demo website"))
					z.Close()
					file.Close()
				}
			} else {
				name += ".sqlite"
				dest += ".sqlite"
				database, e := gorm.Open(sqlite.Open(dest), &gorm.Config{})
				operationErr = e
				if e == nil {
					database.Exec("CREATE TABLE demo (name text)")
					database.Exec("INSERT INTO demo VALUES ('local-only')")
					c, _ := database.DB()
					c.Close()
				}
			}
			data, e := os.ReadFile(dest)
			if e != nil {
				operationErr = e
			}
			sum := sha256.Sum256(data)
			value = shared.BackupResult{Name: name, Kind: kind, Size: int64(len(data)), SHA256: hex.EncodeToString(sum[:]), CreatedAt: time.Now().UnixMilli()}
		default:
			operationErr = fmt.Errorf("本地演示执行器不支持此操作；真实 Linux 客户端提供对应功能")
		}
		data, _ := json.Marshal(value)
		result := shared.ManagementResult{ID: task.ID, Lease: task.Lease, Success: operationErr == nil, Data: data}
		if operationErr != nil {
			result.Error = operationErr.Error()
		}
		code, _ = call("POST", "/api/agent/operations/result", result)
		if code != 200 {
			log.Printf("Local task result rejected: %d", code)
		}
	}
}
