package service

import (
	"cloudease-lite/server/database"
	"cloudease-lite/server/models"
	"golang.org/x/crypto/bcrypt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestV05ConcurrentBootstrapKeepsMatchingPassword(t *testing.T) {
	dir := t.TempDir()
	db, err := database.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	conn, _ := db.DB()
	defer conn.Close()
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); BootstrapAdmin(db, dir, false) }()
	}
	wg.Wait()
	var users []models.User
	if err := db.Find(&users).Error; err != nil || len(users) != 1 {
		t.Fatalf("users=%d %v", len(users), err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "admin-password.txt"))
	if err != nil {
		t.Fatal(err)
	}
	password := strings.TrimSpace(strings.Split(string(raw), "password: ")[1])
	if bcrypt.CompareHashAndPassword([]byte(users[0].Password), []byte(password)) != nil {
		t.Fatal("bootstrap file does not match persisted hash")
	}
	if _, err := os.Stat(filepath.Join(dir, ".admin-initialization.lock")); !os.IsNotExist(err) {
		t.Fatalf("bootstrap lock leaked: %v", err)
	}
}

func TestV05BootstrapRefusesConcurrentInitializer(t *testing.T) {
	dir := t.TempDir()
	db, err := database.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	conn, _ := db.DB()
	defer conn.Close()
	if err := os.WriteFile(filepath.Join(dir, ".admin-initialization.lock"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	created, err := BootstrapAdmin(db, dir, false)
	if created || err == nil {
		t.Fatal("ignored another initializer's lock")
	}
	var count int64
	if err := db.Model(&models.User{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("bootstrap changed users while locked")
	}
}
