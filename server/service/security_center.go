package service

import (
	"bytes"
	"cloudease-lite/server/models"
	"cloudease-lite/shared"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"io"
	"time"
)

func SecurityPolicy(db *gorm.DB, id uint) (models.SecurityRule, shared.SecurityOptions, error) {
	o := shared.DefaultSecurityOptions()
	r := models.SecurityRule{ServerID: id, RetentionDays: 90}
	err := db.First(&r, "server_id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return r, o, nil
	}
	if err != nil {
		return r, o, err
	}
	if json.Unmarshal([]byte(r.OptionsJSON), &o) != nil || !o.Valid() {
		return r, o, errors.New("invalid security rule policy")
	}
	return r, o, nil
}
func QueueSecurity(db *gorm.DB, serverID, userID, sessionID, logID uint, now time.Time) (models.SecurityScan, error) {
	var scan models.SecurityScan
	if serverID == 0 {
		return scan, errors.New("服务器编号无效")
	}
	err := db.Transaction(func(tx *gorm.DB) error {
		var s models.Server
		if e := tx.First(&s, serverID).Error; e != nil {
			return e
		}
		var info shared.ManagementInfo
		_ = json.Unmarshal([]byte(s.ManagementJSON), &info)
		if s.OS != "linux" || !info.Enabled || !info.SecurityEnabled || s.LastSeen == nil || now.Sub(*s.LastSeen) > 90*time.Second {
			return errors.New("客户端离线或未明确启用 v0.4 Linux 安全检测")
		}
		var u models.User
		var session models.Session
		if tx.First(&u, userID).Error != nil || (u.Role != "admin" && u.Role != "operator") || tx.First(&session, "id = ? AND user_id = ? AND expires_at > ?", sessionID, userID, now).Error != nil {
			return errors.New("安全检测授权会话已失效")
		}
		var count int64
		if e := tx.Model(&models.ManagementTask{}).Where("server_id = ? AND status IN ? AND expires_at > ?", serverID, []string{"queued", "running"}, now).Count(&count).Error; e != nil {
			return e
		}
		if count >= 4 {
			return errors.New("运维队列已满")
		}
		if e := tx.Model(&models.SecurityScan{}).Where("server_id = ? AND created_at > ?", serverID, now.Add(-time.Minute)).Count(&count).Error; e != nil {
			return e
		}
		if count > 0 {
			return errors.New("安全扫描每台服务器至少间隔 1 分钟")
		}
		var active int64
		if e := tx.Model(&models.ManagementTask{}).Where("action = ? AND status IN ? AND expires_at > ?", "security.scan", []string{"queued", "running"}, now).Count(&active).Error; e != nil {
			return e
		}
		if active >= 4 {
			return errors.New("全局安全扫描并发已满")
		}
		_, options, e := SecurityPolicy(tx, serverID)
		if e != nil {
			return e
		}
		id, e := RandomToken()
		if e != nil {
			return e
		}
		raw, _ := json.Marshal(shared.ManagementRequest{Action: "security.scan", Security: &options})
		opts, _ := json.Marshal(options)
		task := models.ManagementTask{ID: id, ServerID: serverID, UserID: userID, SessionID: sessionID, OperationLogID: logID, KeyHash: s.Token, Action: "security.scan", RequestJSON: string(raw), Status: "queued", CreatedAt: now, UpdatedAt: now, ExpiresAt: now.Add(45 * time.Second)}
		if e := tx.Create(&task).Error; e != nil {
			return e
		}
		scan = models.SecurityScan{ServerID: serverID, TaskID: id, Status: "queued", OptionsJSON: string(opts), CreatedAt: now, UpdatedAt: now}
		if e := tx.Create(&scan).Error; e != nil {
			return e
		}
		return tx.Model(&models.OperationLog{}).Where("id = ?", logID).Updates(map[string]any{"server_id": serverID, "action": "security.scan", "status": "queued", "updated_at": now}).Error
	})
	return scan, err
}
func securityHash(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }
func securityAlert(tx *gorm.DB, id uint, key, severity, message string, now time.Time) error {
	policy, _, err := SecurityPolicy(tx, id)
	if err != nil {
		return err
	}
	if policy.MuteUntil.After(now) {
		return nil
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&models.SecurityAlert{ServerID: id, Key: securityHash(fmt.Sprint(id) + ":" + key), Severity: severity, Message: shared.SecurityText(message, 700), CreatedAt: now}).Error
}
func FailSecurityScan(tx *gorm.DB, task models.ManagementTask, status, message string, now time.Time) error {
	if task.Action != "security.scan" {
		return nil
	}
	if e := tx.Model(&models.SecurityScan{}).Where("task_id = ? AND status IN ?", task.ID, []string{"queued", "running"}).Updates(map[string]any{"status": status, "error": shared.SecurityText(message, 500), "updated_at": now}).Error; e != nil {
		return e
	}
	return securityAlert(tx, task.ServerID, "failed:"+task.ID, "high", "安全扫描未完成："+status+"；请检查客户端权限、通信与任务期限", now)
}
func StoreSecurityReport(tx *gorm.DB, task models.ManagementTask, data json.RawMessage, now time.Time) error {
	var report shared.SecurityReport
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&report) != nil || decoder.Decode(new(any)) != io.EOF || !report.Valid() {
		return errors.New("invalid security report")
	}
	var scan models.SecurityScan
	if e := tx.First(&scan, "task_id = ? AND server_id = ?", task.ID, task.ServerID).Error; e != nil {
		return e
	}
	var options shared.SecurityOptions
	if json.Unmarshal([]byte(scan.OptionsJSON), &options) != nil || !options.Valid() {
		return errors.New("invalid scan policy")
	}
	var previous models.SecurityScore
	err := tx.Where("server_id = ?", task.ServerID).Order("id desc").First(&previous).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	prior := map[string]bool{}
	eventRules := map[string]bool{}
	for _, ev := range report.Events {
		eventRules[ev.Rule] = true
	}
	if previous.ID != 0 {
		var rows []models.SecurityFinding
		if e := tx.Where("scan_id = ? AND status = ?", previous.ScanID, "risk").Find(&rows).Error; e != nil {
			return e
		}
		for _, f := range rows {
			prior[f.Rule+":"+f.Severity] = true
		}
	}
	for _, c := range report.Checks {
		f := models.SecurityFinding{ServerID: task.ServerID, ScanID: scan.ID, Rule: c.Rule, Dimension: c.Dimension, Title: shared.SecurityText(c.Title, 160), Severity: c.Severity, Status: c.Status, Target: shared.SecurityText(c.Target, 240), Evidence: shared.SecurityText(c.Evidence, 700), Reason: shared.SecurityText(c.Reason, 500), Advice: shared.SecurityText(c.Advice, 900), Workflow: "pending", CreatedAt: now, UpdatedAt: now}
		if e := tx.Create(&f).Error; e != nil {
			return e
		}
		if c.Status == "risk" && (c.Severity == "critical" || c.Severity == "high") && !prior[c.Rule+":"+c.Severity] && !eventRules[c.Rule] {
			if e := securityAlert(tx, task.ServerID, fmt.Sprintf("finding:%d:%s", scan.ID, c.Rule), c.Severity, c.Title+"；"+c.Evidence, now); e != nil {
				return e
			}
		}
	}
	for _, ev := range report.Events {
		observed := time.UnixMilli(ev.ObservedAt).UTC()
		if observed.After(now.Add(5*time.Minute)) || observed.Before(now.Add(-24*time.Hour)) {
			return errors.New("invalid event timestamp")
		}
		row := models.SecurityEvent{ServerID: task.ServerID, Key: ev.Key, Rule: ev.Rule, Severity: ev.Severity, Message: shared.SecurityText(ev.Message, 700), ObservedAt: observed, CreatedAt: now}
		r := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row)
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected > 0 && (ev.Severity == "critical" || ev.Severity == "high") {
			if e := securityAlert(tx, task.ServerID, "event:"+ev.Key, ev.Severity, ev.Message, now); e != nil {
				return e
			}
		}
	}
	grade := shared.GradeSecurity(report.Checks, options)
	raw, _ := json.Marshal(grade)
	var delta *int
	if previous.Score != nil && grade.Score != nil {
		d := *grade.Score - *previous.Score
		delta = &d
	}
	score := models.SecurityScore{ServerID: task.ServerID, ScanID: scan.ID, Score: grade.Score, Risk: grade.Risk, Delta: delta, GradeJSON: string(raw), CreatedAt: now}
	if e := tx.Create(&score).Error; e != nil {
		return e
	}
	if previous.ID != 0 && previous.Risk != grade.Risk {
		if e := securityAlert(tx, task.ServerID, fmt.Sprintf("state:%d", scan.ID), "info", "安全风险状态变化："+previous.Risk+" → "+grade.Risk, now); e != nil {
			return e
		}
	}
	return tx.Model(&scan).Updates(map[string]any{"status": "success", "report_json": string(data), "updated_at": now}).Error
}

// Scheduled scans are owned by an existing authorized user/session. They stop
// being eligible after logout/expiry/demotion; no hidden permanent admin exists.
func TickSecurity(db *gorm.DB, now time.Time) error {
	var policies []models.SecurityRule
	if e := db.Where("interval_hours > 0 AND next_scan_at <= ?", now).Limit(10).Find(&policies).Error; e != nil {
		return e
	}
	for _, p := range policies {
		if e := db.Transaction(func(tx *gorm.DB) error {
			var u models.User
			var s models.Session
			if tx.First(&u, p.UserID).Error != nil || (u.Role != "admin" && u.Role != "operator") || tx.First(&s, "id = ? AND user_id = ? AND expires_at > ?", p.SessionID, p.UserID, now).Error != nil {
				return tx.Model(&p).Update("interval_hours", 0).Error
			}
			entry := models.OperationLog{ServerID: p.ServerID, User: u.Username, Action: "security.schedule", Status: "requested", CreatedAt: now, UpdatedAt: now}
			if e := tx.Create(&entry).Error; e != nil {
				return e
			}
			_, err := QueueSecurity(tx, p.ServerID, p.UserID, p.SessionID, entry.ID, now)
			if err != nil {
				if e := tx.Model(&entry).Update("status", "skipped").Error; e != nil {
					return e
				}
			}
			return tx.Model(&p).Update("next_scan_at", now.Add(time.Duration(p.IntervalHours)*time.Hour)).Error
		}); e != nil {
			return e
		}
	}
	return nil
}
func CleanupSecurity(db *gorm.DB, now time.Time) error {
	var servers []models.Server
	if e := db.Select("id").Find(&servers).Error; e != nil {
		return e
	}
	for _, s := range servers {
		p, _, e := SecurityPolicy(db, s.ID)
		if e != nil {
			return e
		}
		days := p.RetentionDays
		if days < 1 {
			days = 90
		}
		cutoff := now.Add(-time.Duration(days) * 24 * time.Hour)
		for _, model := range []any{&models.SecurityScan{}, &models.SecurityFinding{}, &models.SecurityEvent{}, &models.SecurityAlert{}, &models.SecurityScore{}} {
			if e := db.Where("server_id = ? AND created_at < ?", s.ID, cutoff).Delete(model).Error; e != nil {
				return e
			}
		}
	}
	return db.Where("alert_id NOT IN (?)", db.Model(&models.SecurityAlert{}).Select("id")).Delete(&models.SecurityAlertRead{}).Error
}
