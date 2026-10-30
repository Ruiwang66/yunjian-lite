package security

import (
	"cloudease-lite/shared"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSSHUsesEffectiveConfigurationAndFailureUnknown(t *testing.T) {
	c := New(func(_ context.Context, name string, args ...string) ([]byte, error) {
		if name != "sshd" || strings.Join(args, " ") != "-T" {
			t.Fatal("arbitrary command")
		}
		return []byte("permitrootlogin yes\npasswordauthentication yes\npubkeyauthentication yes\npermitemptypasswords no\nmaxauthtries 7\n"), nil
	})
	items := c.ssh(context.Background())
	if len(items) != 5 || items[4].Severity != "high" || items[4].Status != "risk" {
		t.Fatal(items)
	}
	c.Run = func(context.Context, string, ...string) ([]byte, error) { return nil, os.ErrPermission }
	items = c.ssh(context.Background())
	if items[0].Status != "unknown" {
		t.Fatal("permission failure passed")
	}
}
func TestAddressAndDockerRiskEvidence(t *testing.T) {
	for address, want := range map[string]string{"127.0.0.1:22": "loopback", "[::1]:80": "loopback", "0.0.0.0:22": "wildcard", "[::]:80": "wildcard", "10.1.2.3:3306": "private", "[2001:4860::1]:443": "public_address"} {
		if AddressScope(address) != want {
			t.Fatal(address)
		}
	}
	d := DockerAudit{Name: "test", Image: "alpine:latest", Privileged: true, User: "root", CapAdd: []string{"SYS_ADMIN"}, NetworkMode: "host"}
	d.Mounts = append(d.Mounts, struct {
		Source, Destination string
		RW                  bool
	}{"/var/run/docker.sock", "/var/run/docker.sock", true})
	r := DockerRisks([]DockerAudit{d})
	if r[0].Severity != "critical" || r[1].Severity != "high" || r[0].Evidence == "" {
		t.Fatal("missing evidence")
	}
	c := New(func(context.Context, string, ...string) ([]byte, error) { return nil, exec.ErrNotFound })
	if c.docker(context.Background())[0].Status != "not_installed" {
		t.Fatal("absent docker penalized")
	}
	c.Run = func(context.Context, string, ...string) ([]byte, error) { return nil, os.ErrPermission }
	if c.docker(context.Background())[0].Status != "unknown" {
		t.Fatal("permission failure passed")
	}
}
func TestFullCollectorBoundedReport(t *testing.T) {
	c := New(func(_ context.Context, name string, args ...string) ([]byte, error) {
		if name == "sshd" {
			return []byte("permitrootlogin no\npasswordauthentication no\npubkeyauthentication yes\npermitemptypasswords no\nmaxauthtries 6"), nil
		}
		if name == "ss" {
			return []byte("tcp LISTEN 0 128 0.0.0.0:22 0.0.0.0:* users:((sshd))"), nil
		}
		return nil, errors.New("unavailable")
	})
	c.Read = func(p string) ([]byte, error) {
		if p == "/etc/passwd" {
			return []byte("root:x:0:0::/root:/bin/bash"), nil
		}
		return nil, os.ErrPermission
	}
	c.Stat = func(string) (os.FileInfo, error) { return nil, os.ErrPermission }
	r := c.Scan(context.Background(), shared.DefaultSecurityOptions())
	raw, _ := json.Marshal(r)
	if !r.Valid() || len(raw) > 45000 {
		t.Fatalf("report invalid %d %s", len(raw), raw)
	}
	for _, ch := range r.Checks {
		if ch.Rule == "network.firewall" && ch.Status != "unknown" {
			t.Fatal("missing tool inferred absent firewall")
		}
	}
}
func TestIncrementalLogWindowRotationAndEscaping(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "auth.log")
	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	line := now.Format(time.RFC3339) + " host sshd: Failed password for invalid user <script>alert(1)</script> from 10.0.0.2 port 42 ssh2\n"
	if e := os.WriteFile(path, []byte(strings.Repeat(line, 3)), 0600); e != nil {
		t.Fatal(e)
	}
	r := &LogReader{Sources: []LogSource{{path, "ssh"}}, Now: func() time.Time { return now }}
	o := shared.DefaultSecurityOptions()
	o.SSHFailures = 3
	checks, events := r.Analyze(context.Background(), o)
	if len(events) != 1 || checks[1].Status != "risk" || strings.Contains(events[0].Message, "script") || strings.Contains(events[0].Message, "10.0.0.2") {
		t.Fatal(events)
	}
	_, again := r.Analyze(context.Background(), o)
	if len(again) != 0 {
		t.Fatal("reread produced duplicate event")
	}
	if e := os.Rename(path, path+".1"); e != nil {
		t.Fatal(e)
	}
	os.WriteFile(path, []byte(now.Format(time.RFC3339)+" host sshd: Accepted publickey from 10.0.0.2 port 3\n"), 0600)
	checks, _ = r.Analyze(context.Background(), o)
	if checks[5].Status != "risk" {
		t.Fatal("correlated success missing", checks)
	}
	now = now.Add(time.Hour)
	checks, events = r.Analyze(context.Background(), o)
	if len(events) != 0 || checks[1].Status == "risk" {
		t.Fatal("stale event counted")
	}
}

func BenchmarkSecurityFixtureScan(b *testing.B) {
	c := New(func(context.Context, string, ...string) ([]byte, error) { return nil, exec.ErrNotFound })
	c.Read = func(string) ([]byte, error) { return nil, os.ErrPermission }
	c.Stat = func(string) (os.FileInfo, error) { return nil, os.ErrPermission }
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		report := c.Scan(context.Background(), shared.DefaultSecurityOptions())
		if !report.Valid() {
			b.Fatal("invalid report")
		}
	}
}
