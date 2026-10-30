package management

import (
	"cloudease-lite/shared"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSecurityExplicitAuthorizationAndNoTaskParameters(t *testing.T) {
	m := &Manager{platform: "linux", Run: func(context.Context, string, ...string) ([]byte, error) { return nil, os.ErrPermission }}
	if m.ConfigureSecurity(true, "") == nil {
		t.Fatal("ordinary monitor gained security")
	}
	o := shared.DefaultSecurityOptions()
	if _, e := m.Execute(context.Background(), shared.ManagementRequest{Action: "security.scan", Security: &o}); e == nil {
		t.Fatal("ordinary monitor executed")
	}
	m.Enabled = true
	if e := m.ConfigureSecurity(true, ""); e != nil {
		t.Fatal(e)
	}
	for _, req := range []shared.ManagementRequest{{Action: "security.scan", Security: &o, Path: "../../etc/shadow"}, {Action: "security.scan", Security: &o, Target: ";id"}, {Action: "security.scan", Security: &o, Content: "echo hi"}, {Action: "security.scan"}} {
		if _, e := m.Execute(context.Background(), req); e == nil {
			t.Fatal("extra parameter accepted")
		}
	}
	m.platform = "windows"
	if m.ConfigureSecurity(true, "") == nil {
		t.Fatal("Windows security enabled")
	}
}

func TestSecurityWebsiteMetadataAndRedirectTargets(t *testing.T) {
	dir := t.TempDir()
	if mPathLinux() && secureRoot(dir) != nil {
		t.Skip("requires root-owned protected parent")
	}
	if e := os.WriteFile(filepath.Join(dir, "backup.sql"), []byte("PRIVATE CONTENT MUST NOT BE READ"), 0600); e != nil {
		t.Fatal(e)
	}
	files, complete := securitySensitiveEntries(dir)
	if !complete || len(files) != 1 || files[0] != "backup.sql" || strings.Contains(strings.Join(files, ""), "PRIVATE") {
		t.Fatal("metadata-only audit failed")
	}
	for _, check := range []shared.WebsiteCheck{{Domain: "localhost"}, {Domain: "example.com", A: []string{"127.0.0.1"}}, {Domain: "other.example.com"}} {
		check.CheckedAt = time.Now().UnixMilli()
		if securityRedirect(context.Background(), "example.com", check) != "unknown" {
			t.Fatal("unsafe redirect target accepted")
		}
	}
}
