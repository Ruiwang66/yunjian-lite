package database

import (
	"cloudease-lite/server/models"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"path/filepath"
	"testing"
)

func TestV01MigrationPreservesUsersServersSessions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cloudease.db")
	old, err := gorm.Open(sqlite.Open(path))
	if err != nil {
		t.Fatal(err)
	}
	queries := []string{
		`CREATE TABLE users (id integer PRIMARY KEY, username text, password text, created_at datetime)`,
		`INSERT INTO users VALUES (1, 'admin', 'existing-bcrypt-hash', '2026-10-10 00:00:00')`,
		`CREATE TABLE sessions (id integer PRIMARY KEY, token_hash text, user_id integer, expires_at datetime)`,
		`INSERT INTO sessions VALUES (1,'existing-session-hash',1,'2027-01-01 00:00:00')`,
		`CREATE TABLE servers (id integer PRIMARY KEY, name text, token text, status text)`,
		`INSERT INTO servers VALUES (1,'existing-server','existing-key-hash','pending')`,
	}
	for _, q := range queries {
		if err := old.Exec(q).Error; err != nil {
			t.Fatal(err)
		}
	}
	sqlDB, _ := old.DB()
	sqlDB.Close()
	for i := 0; i < 2; i++ {
		db, err := Open(dir)
		if err != nil {
			t.Fatal(err)
		}
		var u models.User
		var s models.Server
		var session models.Session
		if db.First(&u, 1).Error != nil || u.Password != "existing-bcrypt-hash" || u.Role != "admin" {
			t.Fatalf("user migration: %+v", u)
		}
		if db.First(&s, 1).Error != nil || s.Name != "existing-server" || s.Token != "existing-key-hash" {
			t.Fatalf("server migration: %+v", s)
		}
		if db.First(&session, 1).Error != nil || session.TokenHash != "existing-session-hash" {
			t.Fatal("session lost")
		}
		for _, table := range []string{"roles", "operation_logs", "docker_containers", "services", "files", "management_tasks"} {
			if !db.Migrator().HasTable(table) {
				t.Fatalf("missing table %s", table)
			}
		}
		var count int64
		db.Model(&models.Role{}).Count(&count)
		if count != 3 {
			t.Fatalf("roles not seeded idempotently: %d", count)
		}
		conn, _ := db.DB()
		conn.Close()
	}
}
