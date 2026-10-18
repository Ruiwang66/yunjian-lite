package management

import (
	"archive/zip"
	"cloudease-lite/shared"
	"context"
	"encoding/json"
	"errors"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProbeRejectsNonPublicAddresses(t *testing.T) {
	for _, ip := range []string{"127.0.0.1", "10.0.0.1", "172.16.0.1", "192.168.0.1", "100.64.1.1", "169.254.169.254", "::1", "fc00::1", "fe80::1", "192.0.2.1", "64:ff9b::a00:1"} {
		if publicAddress(net.ParseIP(ip)) {
			t.Fatal(ip)
		}
	}
	if !publicAddress(net.ParseIP("8.8.8.8")) {
		t.Fatal("public IP rejected")
	}
	result := probeWebsite(context.Background(), "localhost")
	if result.Accessible || result.Error == "" {
		t.Fatal(result)
	}
}
func TestWebsiteAllowlistAndRead(t *testing.T) {
	dir := t.TempDir()
	site := SiteConfig{WebsiteSpec: shared.WebsiteSpec{Name: "博客", Domain: "blog.example.com", RootPath: "/srv/blog", NginxConfig: "blog.conf"}}
	os.WriteFile(filepath.Join(dir, "blog.conf"), []byte("server { autoindex on; }\nproxy_set_header Authorization SECRET;\n"), 0600)
	m := &Manager{Enabled: true, platform: "linux", privileged: true, Roots: []string{dir}, websites: &websiteManager{config: WebsiteConfig{AvailableDir: dir, EnabledDir: dir, Sites: []SiteConfig{site}}}}
	if mPathLinux() && secureRoot(dir) != nil {
		t.Skip("Linux test directory requires secure root-owned ancestors")
	}
	spec := site.WebsiteSpec
	data, err := m.Execute(context.Background(), shared.ManagementRequest{Action: "nginx.read", Target: site.Domain, Website: &spec})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(data)
	if strings.Contains(string(raw), "SECRET") {
		t.Fatal("secret header leaked")
	}
	spec.RootPath = "/srv/other"
	if _, err = m.Execute(context.Background(), shared.ManagementRequest{Action: "nginx.read", Target: site.Domain, Website: &spec}); err == nil {
		t.Fatal("allowlist bypass")
	}
	if _, err = m.Execute(context.Background(), shared.ManagementRequest{Action: "nginx.status", Content: "injection"}); err == nil {
		t.Fatal("extraneous args")
	}
}
func TestNginxFailureRollsBack(t *testing.T) {
	if mPathLinux() {
		t.Skip("requires root-owned fixture parents on Linux")
	}
	dir := t.TempDir()
	available := filepath.Join(dir, "available")
	enabled := filepath.Join(dir, "enabled")
	backup := filepath.Join(dir, "backup")
	for _, d := range []string{available, enabled, backup} {
		os.Mkdir(d, 0700)
	}
	site := SiteConfig{WebsiteSpec: shared.WebsiteSpec{Domain: "blog.example.com", NginxConfig: "blog.conf"}}
	os.WriteFile(filepath.Join(available, "blog.conf"), []byte("server {}"), 0600)
	probe := filepath.Join(enabled, "probe")
	if err := os.Symlink(filepath.Join(available, "blog.conf"), probe); err != nil {
		t.Skip("symlink creation unavailable")
	}
	os.Remove(probe)
	m := &Manager{privileged: true, websites: &websiteManager{config: WebsiteConfig{AvailableDir: available, EnabledDir: enabled, BackupDir: backup}}, Run: func(ctx context.Context, name string, args ...string) ([]byte, error) {
		if name == "nginx" && len(args) == 1 && args[0] == "-t" {
			return nil, errors.New("invalid config")
		}
		t.Fatal("unexpected command", name, args)
		return nil, nil
	}}
	if _, err := m.changeSite(context.Background(), site, true); err == nil {
		t.Fatal("expected rejection")
	}
	if _, err := os.Lstat(filepath.Join(enabled, "blog.conf")); !os.IsNotExist(err) {
		t.Fatal("link not rolled back")
	}
	files, _ := os.ReadDir(backup)
	if len(files) != 1 {
		t.Fatal("config backup missing")
	}
}
func TestWebsiteFileAndSQLiteBackup(t *testing.T) {
	if mPathLinux() {
		t.Skip("requires root-owned fixture parents on Linux")
	}
	dir := t.TempDir()
	siteRoot := filepath.Join(dir, "site")
	backupDir := filepath.Join(dir, "backup")
	dbDir := filepath.Join(dir, "db")
	for _, d := range []string{siteRoot, backupDir, dbDir} {
		os.Mkdir(d, 0700)
	}
	os.WriteFile(filepath.Join(siteRoot, "index.html"), []byte("hello website"), 0600)
	os.Mkdir(filepath.Join(siteRoot, ".git"), 0700)
	os.WriteFile(filepath.Join(siteRoot, ".git", "config"), []byte("private"), 0600)
	m := &Manager{websites: &websiteManager{config: WebsiteConfig{BackupDir: backupDir}}}
	site := SiteConfig{WebsiteSpec: shared.WebsiteSpec{Domain: "blog.example.com", RootPath: siteRoot}}
	result, err := m.backupSite(context.Background(), site, "files")
	if err != nil {
		t.Fatal(err)
	}
	if result.Size <= 0 || len(result.SHA256) != 64 {
		t.Fatal(result)
	}
	reader, err := zip.OpenReader(filepath.Join(backupDir, result.Name))
	if err != nil {
		t.Fatal(err)
	}
	if len(reader.File) != 1 || reader.File[0].Name != "index.html" {
		t.Fatal("archive includes excluded paths")
	}
	reader.Close()
	site.SQLitePath = filepath.Join(dbDir, "site.sqlite")
	db, err := gorm.Open(sqlite.Open(site.SQLitePath), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	db.Exec("CREATE TABLE test (name text)")
	db.Exec("INSERT INTO test VALUES ('preserved')")
	conn, _ := db.DB()
	conn.Close()
	os.Chmod(site.SQLitePath, 0600)
	result, err = m.backupSite(context.Background(), site, "database")
	if err != nil {
		t.Fatal(err)
	}
	copy, e := gorm.Open(sqlite.Open(filepath.Join(backupDir, result.Name)), &gorm.Config{})
	if e != nil {
		t.Fatal(e)
	}
	var name string
	copy.Raw("SELECT name FROM test").Scan(&name)
	c, _ := copy.DB()
	c.Close()
	if name != "preserved" {
		t.Fatal("database backup inconsistent")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = m.backupSite(ctx, site, "files"); err == nil {
		t.Fatal("cancelled backup succeeded")
	}
	files, _ := os.ReadDir(backupDir)
	for _, f := range files {
		if strings.HasPrefix(f.Name(), ".partial-") {
			t.Fatal("partial backup leaked")
		}
	}
}
