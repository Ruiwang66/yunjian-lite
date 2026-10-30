package api

import (
	"cloudease-lite/server/models"
	"cloudease-lite/server/service"
	"cloudease-lite/shared"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

func securityFixture(t *testing.T) *managementFixture {
	f := newManagementFixture(t)
	var s models.Server
	f.db.First(&s, 1)
	var m shared.ManagementInfo
	json.Unmarshal([]byte(s.ManagementJSON), &m)
	m.SecurityEnabled = true
	raw, _ := json.Marshal(m)
	if e := f.db.Model(&s).Update("management_json", string(raw)).Error; e != nil {
		t.Fatal(e)
	}
	return f
}
func startSecurity(t *testing.T, f *managementFixture) models.SecurityScan {
	t.Helper()
	w := f.call("POST", "/api/security/scans", map[string]any{"server_id": 1}, "operator", "")
	if w.Code != 202 {
		t.Fatalf("queue %d %s", w.Code, w.Body.String())
	}
	var s models.SecurityScan
	json.Unmarshal(w.Body.Bytes(), &s)
	return s
}
func completeSecurity(t *testing.T, f *managementFixture, scan models.SecurityScan) shared.ManagementResult {
	t.Helper()
	lease := f.lease()
	if lease.ID != scan.TaskID || lease.Request.Action != "security.scan" || lease.Request.Security == nil {
		t.Fatal("invalid typed task")
	}
	now := time.Now()
	data, _ := json.Marshal(shared.SecurityReport{StartedAt: now.UnixMilli(), FinishedAt: now.UnixMilli(), Scope: "test fixture only", Checks: []shared.SecurityCheck{{Rule: "docker.privileged", Dimension: "docker", Title: "特权容器", Status: "risk", Severity: "critical", Evidence: "fixture", Advice: "人工核对"}, {Rule: "ssh.unavailable", Dimension: "network", Title: "无法读取", Status: "unknown", Severity: "info"}}, Events: []shared.SecurityObservation{{Key: strings.Repeat("a", 64), Rule: "logs.ssh_failures", Severity: "high", Message: "fixture anomaly", ObservedAt: now.UnixMilli()}}})
	result := shared.ManagementResult{ID: lease.ID, Lease: lease.Lease, Success: true, Data: data}
	w := f.call("POST", "/api/agent/operations/result", result, "", f.key)
	if w.Code != 200 {
		t.Fatalf("result %d %s", w.Code, w.Body.String())
	}
	return result
}
func TestSecurityRBACAndForbiddenTaskInjection(t *testing.T) {
	f := securityFixture(t)
	for _, p := range []string{"overview", "scans", "findings", "events", "alerts", "scores", "rules", "reports/1", "findings/1"} {
		path := "/api/security/" + p + "?server_id=1"
		if f.call("GET", path, nil, "", "").Code != 401 {
			t.Fatal("unauthenticated read", p)
		}
		if f.call("GET", path, nil, "viewer", "").Code != 403 {
			t.Fatal("viewer security read", p)
		}
	}
	if f.call("POST", "/api/security/scans", map[string]any{"server_id": 1}, "viewer", "").Code != 403 {
		t.Fatal("viewer scan")
	}
	if f.call("POST", "/api/security/scans", map[string]any{"server_id": 1, "path": "../../etc/shadow", "command": "id"}, "operator", "").Code != 400 {
		t.Fatal("arbitrary task params")
	}
	if validManagementRequest(shared.ManagementRequest{Action: "security.scan"}) {
		t.Fatal("generic management route bypass")
	}
	if f.call("POST", "/api/security/scans", map[string]any{"server_id": 999}, "operator", "").Code != 409 {
		t.Fatal("unknown server queued")
	}
	for _, q := range []string{"days=9999", "page=-1", "severity=high%27%20OR%201=1"} {
		if f.call("GET", "/api/security/findings?server_id=1&"+q, nil, "admin", "").Code != 400 {
			t.Fatal("unsafe query", q)
		}
	}
	var s models.Server
	f.db.First(&s, 1)
	f.db.Model(&s).Update("management_json", `{"enabled":false,"security_enabled":true}`)
	if f.call("POST", "/api/security/scans", map[string]any{"server_id": 1}, "operator", "").Code != 409 {
		t.Fatal("compatibility mode gained capability")
	}
}
func TestSecurityScanReportAlertsAndImmutableGrade(t *testing.T) {
	f := securityFixture(t)
	scan := startSecurity(t, f)
	result := completeSecurity(t, f, scan)
	if f.call("POST", "/api/agent/operations/result", result, "", f.key).Code != 409 {
		t.Fatal("result replay accepted")
	}
	var n int64
	f.db.Model(&models.SecurityScore{}).Count(&n)
	if n != 1 {
		t.Fatal("replay changed score")
	}
	var score models.SecurityScore
	f.db.First(&score)
	if score.Risk != "critical" || score.Score == nil || *score.Score != 40 {
		t.Fatal(score)
	}
	var finding models.SecurityFinding
	f.db.First(&finding, "rule = ?", "docker.privileged")
	w := f.call("PATCH", fmt.Sprint("/api/security/findings/", finding.ID), map[string]any{"server_id": 1, "status": "false_positive"}, "operator", "")
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	f.db.First(&score)
	if *score.Score != 40 {
		t.Fatal("workflow hid risk")
	}
	f.db.First(&finding, finding.ID)
	if finding.Evidence != "fixture" || finding.Status != "risk" || finding.Workflow != "false_positive" {
		t.Fatal("original evidence overwritten")
	}
	path := fmt.Sprintf("/api/security/reports/%d?server_id=1", scan.ID)
	if w := f.call("GET", path, nil, "operator", ""); w.Code != 200 || !strings.Contains(w.Body.String(), "test fixture only") {
		t.Fatal("report missing")
	}
	f.db.Create(&models.Server{Name: "other", Token: service.HashToken(strings.Repeat("B", 43))})
	if f.call("GET", fmt.Sprintf("/api/security/reports/%d?server_id=2", scan.ID), nil, "operator", "").Code != 404 {
		t.Fatal("cross-server report IDOR")
	}
	if f.call("GET", "/api/agent/operations/next", nil, "", strings.Repeat("B", 43)).Code != 204 {
		t.Fatal("cross-server delivery")
	}
	if f.call("GET", "/api/operations/"+scan.TaskID, nil, "viewer", "").Code != 403 {
		t.Fatal("viewer bypass via operation result")
	}
	f.db.Model(&models.SecurityScan{}).Where("id = ?", scan.ID).Update("created_at", time.Now().UTC().Add(-2*time.Minute))
	scan2 := startSecurity(t, f)
	completeSecurity(t, f, scan2)
	f.db.Model(&models.SecurityEvent{}).Count(&n)
	if n != 1 {
		t.Fatal("duplicate event stored")
	}
	f.db.Model(&models.SecurityAlert{}).Count(&n)
	if n != 2 {
		t.Fatal("duplicate continuous risk alerted", n)
	}
	var alert models.SecurityAlert
	f.db.First(&alert)
	if w := f.call("PATCH", fmt.Sprint("/api/security/alerts/", alert.ID), map[string]any{"server_id": 1}, "operator", ""); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if w := f.call("GET", "/api/security/alerts?server_id=1", nil, "admin", ""); w.Code != 200 || !strings.Contains(w.Body.String(), `"read":[]`) {
		t.Fatal("read state leaked across users", w.Body.String())
	}
}
func TestSecurityFailureSilenceScheduleAndRetention(t *testing.T) {
	f := securityFixture(t)
	opts := shared.DefaultSecurityOptions()
	body := map[string]any{"server_id": 1, "options": opts, "retention_days": 30, "mute_hours": 1, "interval_hours": 6}
	if f.call("PUT", "/api/security/rules", body, "operator", "").Code != 403 {
		t.Fatal("operator changed score weights")
	}
	if w := f.call("PUT", "/api/security/rules", body, "admin", ""); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	scan := startSecurity(t, f)
	lease := f.lease()
	if w := f.call("POST", "/api/agent/operations/result", shared.ManagementResult{ID: lease.ID, Lease: lease.Lease, Error: "permission denied"}, "", f.key); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	f.db.First(&scan, scan.ID)
	if scan.Status != "failed" {
		t.Fatal("failure not persisted")
	}
	var n int64
	f.db.Model(&models.SecurityAlert{}).Count(&n)
	if n != 0 {
		t.Fatal("silence ignored")
	}
	f.db.Model(&models.SecurityRule{}).Where("server_id = ?", 1).Update("next_scan_at", time.Now().UTC().Add(-time.Minute))
	f.call("POST", "/api/auth/logout", nil, "admin", "")
	if e := service.TickSecurity(f.db, time.Now().UTC()); e != nil {
		t.Fatal(e)
	}
	var rule models.SecurityRule
	f.db.First(&rule, 1)
	if rule.IntervalHours != 0 {
		t.Fatal("schedule outlived authorization")
	}
	old := time.Now().UTC().Add(-31 * 24 * time.Hour)
	f.db.Create(&models.SecurityEvent{ServerID: 1, Key: "old", CreatedAt: old})
	f.db.Create(&models.Metric{ServerID: 1, CreatedAt: old})
	var before int64
	f.db.Model(&models.Metric{}).Count(&before)
	if e := service.CleanupSecurity(f.db, time.Now().UTC()); e != nil {
		t.Fatal(e)
	}
	f.db.Model(&models.SecurityEvent{}).Where("key = ?", "old").Count(&n)
	if n != 0 {
		t.Fatal("security retention ignored")
	}
	f.db.Model(&models.Metric{}).Count(&n)
	if n != before {
		t.Fatal("security cleanup changed monitoring data")
	}
}

func TestSecurityExpiredAndForgedAgentResult(t *testing.T) {
	f := securityFixture(t)
	scan := startSecurity(t, f)
	lease := f.lease()
	forged := shared.ManagementResult{ID: lease.ID, Lease: strings.Repeat("Z", 43), Success: true, Data: json.RawMessage(`{}`)}
	if f.call("POST", "/api/agent/operations/result", forged, "", f.key).Code != 409 {
		t.Fatal("forged lease accepted")
	}
	if f.call("POST", "/api/agent/operations/result", forged, "", strings.Repeat("X", 43)).Code != 401 {
		t.Fatal("forged client accepted")
	}
	now := time.Now().UTC()
	f.db.Model(&models.ManagementTask{}).Where("id = ?", lease.ID).Update("expires_at", now.Add(-time.Second))
	if e := service.ExpireOperations(f.db, now); e != nil {
		t.Fatal(e)
	}
	f.db.First(&scan, scan.ID)
	if scan.Status != "expired" {
		t.Fatal("offline/expired state missing")
	}
	var count int64
	f.db.Model(&models.SecurityAlert{}).Count(&count)
	if count != 1 {
		t.Fatal("failure notification missing")
	}
	var m models.ManagementTask
	f.db.First(&m, "id = ?", lease.ID)
	if m.Status != "expired" {
		t.Fatal("lease not expired")
	}
}

func TestSecurityMultipleServersKeepTasksAndResultsSeparate(t *testing.T) {
	f := securityFixture(t)
	now := time.Now().UTC()
	key := strings.Repeat("B", 43)
	raw, _ := json.Marshal(shared.ManagementInfo{Enabled: true, SecurityEnabled: true, Version: shared.Version})
	other := models.Server{Name: "second", Token: service.HashToken(key), OS: "linux", LastSeen: &now, ManagementJSON: string(raw)}
	if e := f.db.Create(&other).Error; e != nil {
		t.Fatal(e)
	}
	first := startSecurity(t, f)
	w := f.call("POST", "/api/security/scans", map[string]any{"server_id": other.ID}, "operator", "")
	if w.Code != 202 {
		t.Fatal(w.Body.String())
	}
	var second models.SecurityScan
	json.Unmarshal(w.Body.Bytes(), &second)
	lease1 := f.lease()
	w = f.call("GET", "/api/agent/operations/next", nil, "", key)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var lease2 shared.ManagementTask
	json.Unmarshal(w.Body.Bytes(), &lease2)
	if lease1.ID != first.TaskID || lease2.ID != second.TaskID || lease1.ID == lease2.ID {
		t.Fatal("cross-host task delivery")
	}
	data, _ := json.Marshal(shared.SecurityReport{StartedAt: now.UnixMilli(), FinishedAt: now.UnixMilli(), Checks: []shared.SecurityCheck{{Rule: "system.fixture", Dimension: "system", Title: "fixture", Status: "pass", Severity: "info"}}, Scope: "unit test"})
	if f.call("POST", "/api/agent/operations/result", shared.ManagementResult{ID: lease2.ID, Lease: lease2.Lease, Success: true, Data: data}, "", f.key).Code != 404 {
		t.Fatal("cross-host result accepted")
	}
	for i, lease := range []shared.ManagementTask{lease1, lease2} {
		k := f.key
		if i == 1 {
			k = key
		}
		if w := f.call("POST", "/api/agent/operations/result", shared.ManagementResult{ID: lease.ID, Lease: lease.Lease, Success: true, Data: data}, "", k); w.Code != 200 {
			t.Fatal(w.Body.String())
		}
	}
	for _, id := range []uint{1, other.ID} {
		var count int64
		f.db.Model(&models.SecurityScore{}).Where("server_id = ?", id).Count(&count)
		if count != 1 {
			t.Fatal("wrong result ownership")
		}
	}
}

func TestSecurityLogFindingAndEventOnlyNotifyOnce(t *testing.T) {
	f := securityFixture(t)
	startSecurity(t, f)
	lease := f.lease()
	now := time.Now().UnixMilli()
	data, _ := json.Marshal(shared.SecurityReport{StartedAt: now, FinishedAt: now, Checks: []shared.SecurityCheck{{Rule: "logs.ssh_failures", Dimension: "logs", Title: "失败频率", Status: "risk", Severity: "high"}}, Events: []shared.SecurityObservation{{Key: strings.Repeat("d", 64), Rule: "logs.ssh_failures", Severity: "high", Message: "失败频率", ObservedAt: now}}})
	if w := f.call("POST", "/api/agent/operations/result", shared.ManagementResult{ID: lease.ID, Lease: lease.Lease, Success: true, Data: data}, "", f.key); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var count int64
	f.db.Model(&models.SecurityAlert{}).Count(&count)
	if count != 1 {
		t.Fatal("same finding/event produced duplicate notifications", count)
	}
}
