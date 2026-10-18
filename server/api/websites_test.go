package api

import (
	"cloudease-lite/server/models"
	"cloudease-lite/shared"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"
)

func TestWebsiteManagementLifecycle(t *testing.T) {
	f := newManagementFixture(t)
	var server models.Server
	f.db.First(&server)
	spec := shared.WebsiteSpec{Name: "博客", Domain: "blog.example.com", RootPath: "/srv/blog", NginxConfig: "blog.conf"}
	var info shared.ManagementInfo
	json.Unmarshal([]byte(server.ManagementJSON), &info)
	info.Websites = &shared.WebsiteInventory{Enabled: true, Sites: []shared.WebsiteSpec{spec}, PHP: shared.PHPInfo{Version: "PHP 8.2"}}
	raw, _ := json.Marshal(info)
	f.db.Model(&server).Update("management_json", string(raw))
	body := map[string]any{"server_id": server.ID, "name": spec.Name, "domain": spec.Domain, "root_path": spec.RootPath, "nginx_config": spec.NginxConfig}
	if w := f.call("POST", "/api/websites", body, "viewer", ""); w.Code != 403 {
		t.Fatal(w.Code)
	}
	if w := f.call("GET", fmt.Sprintf("/api/websites?server_id=%d", server.ID), nil, "", ""); w.Code != 401 {
		t.Fatal(w.Code)
	}
	bad := map[string]any{"server_id": server.ID, "name": "bad", "domain": "bad.example.com", "root_path": "/etc", "nginx_config": "evil.conf"}
	if w := f.call("POST", "/api/websites", bad, "admin", ""); w.Code != 403 {
		t.Fatal(w.Code, w.Body.String())
	}
	w := f.call("POST", "/api/websites", body, "operator", "")
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	var website models.Website
	json.Unmarshal(w.Body.Bytes(), &website)
	body["name"] = "新名称"
	w = f.call("PATCH", fmt.Sprintf("/api/websites/%d", website.ID), body, "admin", "")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	id := f.task("POST", fmt.Sprintf("/api/websites/%d/check", website.ID), map[string]any{"server_id": server.ID, "website_id": website.ID}, "operator")
	task := f.lease()
	if task.ID != id || task.Request.Website.Domain != spec.Domain {
		t.Fatal(task)
	}
	check := shared.WebsiteCheck{Domain: spec.Domain, Status: "正常", HTTPStatus: 200, ResponseMS: 220, Accessible: true, A: []string{"8.8.8.8"}, CheckedAt: time.Now().UnixMilli(), TLS: shared.TLSInfo{Status: "有效", DaysLeft: 6, Warning: "高危：证书将在 7 天内过期", Version: "TLS 1.3"}}
	data, _ := json.Marshal(check)
	result := shared.ManagementResult{ID: task.ID, Lease: task.Lease, Success: true, Data: data}
	w = f.call("POST", "/api/agent/operations/result", result, "", f.key)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var ssl models.SSLCertificate
	f.db.Where("website_id = ?", website.ID).First(&ssl)
	if ssl.DaysLeft != 6 {
		t.Fatal("certificate not persisted")
	}
	var health models.HealthCheck
	f.db.Where("website_id = ?", website.ID).First(&health)
	if health.ResponseMS != 220 {
		t.Fatal("health not persisted")
	}
	for _, kind := range []string{"files", "database"} {
		id = f.task("POST", "/api/backup", map[string]any{"server_id": server.ID, "website_id": website.ID, "kind": kind}, "admin")
		task = f.lease()
		if task.Request.Action != "backup."+kind {
			t.Fatal("backup route closure retained previous kind")
		}
		data, _ = json.Marshal(shared.BackupResult{Name: "backup." + kind, Kind: kind, Size: 12, SHA256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"})
		w = f.call("POST", "/api/agent/operations/result", shared.ManagementResult{ID: id, Lease: task.Lease, Success: true, Data: data}, "", f.key)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	if w = f.call("GET", fmt.Sprintf("/api/backup?server_id=%d", server.ID), nil, "viewer", ""); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if w = f.call("DELETE", fmt.Sprintf("/api/websites/%d", website.ID), nil, "admin", ""); w.Code != http.StatusOK {
		t.Fatal(w.Code, w.Body.String())
	}
	var count int64
	f.db.Model(&models.BackupTask{}).Count(&count)
	if count != 2 {
		t.Fatal("backup history deleted")
	}
	f.db.Model(&models.OperationLog{}).Where("action = ?", "website.check").Count(&count)
	if count != 1 {
		t.Fatal("audit missing")
	}
}
