package database

import (
	"cloudease-lite/server/models"
	"cloudease-lite/shared"
	"encoding/json"
	"testing"
	"time"
)

// v0.5 changes no schema fields. Reopening an existing v0.4-shaped snapshot
// must preserve credentials, historical records and custom retention policy.
func TestV05RepeatedOpenPreservesExistingWorkspaceData(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	u := models.User{Username: "existing-viewer", Role: "viewer", Password: "existing-bcrypt-hash"}
	s := models.Server{Name: "existing-server", Token: "existing-agent-hash", CountryCode: "CN", Status: "offline"}
	if err := db.Create(&u).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&s).Error; err != nil {
		t.Fatal(err)
	}
	options, _ := json.Marshal(shared.DefaultSecurityOptions())
	for _, row := range []any{
		&models.Session{UserID: u.ID, TokenHash: "existing-session-hash", ExpiresAt: now.Add(time.Hour)},
		&models.Metric{ServerID: s.ID, CPU: 12, CreatedAt: now.Add(-31 * 24 * time.Hour)},
		&models.SystemLog{ServerID: s.ID, Message: "existing history", CreatedAt: now.Add(-31 * 24 * time.Hour)},
		&models.BackupTask{ServerID: s.ID, TaskID: "existing-backup", Name: "existing.zip", Status: "success"},
		&models.SecurityRule{ServerID: s.ID, RetentionDays: 120, OptionsJSON: string(options)},
	} {
		if err := db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	conn, _ := db.DB()
	conn.Close()
	for i := 0; i < 2; i++ {
		db, err = Open(dir)
		if err != nil {
			t.Fatal(err)
		}
		var user models.User
		var server models.Server
		var policy models.SecurityRule
		if err := db.First(&user, u.ID).Error; err != nil || user.Role != "viewer" || user.Password != "existing-bcrypt-hash" {
			t.Fatal("user changed", err)
		}
		if err := db.First(&server, s.ID).Error; err != nil || server.Token != "existing-agent-hash" || server.CountryCode != "CN" {
			t.Fatal("server changed", err)
		}
		if err := db.First(&policy, "server_id = ?", s.ID).Error; err != nil || policy.RetentionDays != 120 || policy.OptionsJSON != string(options) {
			t.Fatal("policy changed", err)
		}
		for _, model := range []any{&models.Session{}, &models.Metric{}, &models.SystemLog{}, &models.BackupTask{}} {
			var count int64
			if err := db.Model(model).Count(&count).Error; err != nil || count != 1 {
				t.Fatalf("%T lost rows: %d %v", model, count, err)
			}
		}
		conn, _ = db.DB()
		conn.Close()
	}
}
