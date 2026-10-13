package management

import (
	"cloudease-lite/shared"
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func fixture(t *testing.T) *Manager {
	t.Helper()
	root := t.TempDir()
	if runtime.GOOS == "linux" { // os.Root file tests do not need to exercise installation-policy checks.
		root = filepath.Join("/opt", "cloudease-v02-test-"+filepath.Base(filepath.Dir(root))+"-"+filepath.Base(root))
		if os.Geteuid() != 0 {
			return &Manager{Enabled: true, Roots: []string{t.TempDir()}, Services: map[string]bool{"nginx.service": true}, privileged: true, platform: "linux"}
		}
		if err := os.Mkdir(root, 0700); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { os.RemoveAll(root) })
	}
	return &Manager{Enabled: true, Roots: []string{root}, Services: map[string]bool{"nginx.service": true}, privileged: true, platform: "linux"}
}
func TestWhitelistedOperationsNeverAcceptShell(t *testing.T) {
	m := fixture(t)
	calls := [][]string{}
	m.Run = func(_ context.Context, name string, args ...string) ([]byte, error) {
		calls = append(calls, append([]string{name}, args...))
		return []byte("ok"), nil
	}
	ctx := context.Background()
	bad := []shared.ManagementRequest{{Action: "command.rm"}, {Action: "command.disk", Target: "; rm -rf /"}, {Action: "docker.start", Target: "abc;whoami"}, {Action: "service.restart", Target: "sshd"}, {Action: "service.restart", Target: "systemd-journald"}, {Action: "service.restart", Target: "nginx;id"}, {Action: "service.restart", Target: "mysql"}, {Action: "command.disk", Path: "/etc"}, {Action: "file.upload", Path: "../escape.txt"}}
	for _, req := range bad {
		if _, err := m.Execute(ctx, req); err == nil {
			t.Fatalf("unsafe request accepted: %+v", req)
		}
	}
	if len(calls) != 0 {
		t.Fatal("invalid requests reached command runner")
	}
	if _, err := m.Execute(ctx, shared.ManagementRequest{Action: "service.restart", Target: "nginx"}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Execute(ctx, shared.ManagementRequest{Action: "docker.stop", Target: strings.Repeat("a", 64)}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Execute(ctx, shared.ManagementRequest{Action: "command.disk"}); err != nil {
		t.Fatal(err)
	}
	expected := [][]string{{"systemctl", "restart", "nginx.service"}, {"docker", "stop", "--time", "5", strings.Repeat("a", 64)}, {"df", "-h"}}
	if !reflect.DeepEqual(calls, expected) {
		t.Fatalf("unexpected argv: %v", calls)
	}
	m.privileged = false
	if _, err := m.Execute(ctx, shared.ManagementRequest{Action: "service.restart", Target: "nginx"}); err == nil {
		t.Fatal("unprivileged service write accepted")
	}
	m.Enabled = false
	if _, err := m.Execute(ctx, shared.ManagementRequest{Action: "command.disk"}); err == nil {
		t.Fatal("disabled management accepted")
	}
}
func TestDockerAndSystemdParsing(t *testing.T) {
	m := fixture(t)
	m.Run = func(_ context.Context, name string, args ...string) ([]byte, error) {
		switch name + " " + args[0] {
		case "docker info":
			return []byte(`{"ServerVersion":"27.5.1","Containers":2,"Images":4}`), nil
		case "docker ps":
			return []byte(`{"ID":"aaaaaaaaaaaa","Names":"web","State":"running","Ports":"80/tcp","CreatedAt":"2026-10-11"}`), nil
		case "systemctl list-unit-files":
			return []byte("nginx.service enabled enabled\nsshd.service enabled enabled\n"), nil
		case "systemctl list-units":
			return []byte("nginx.service loaded active running Web server\nsshd.service loaded active running SSH\n"), nil
		}
		t.Fatalf("unexpected command: %s %v", name, args)
		return nil, nil
	}
	info, err := m.dockerInfo(context.Background())
	if err != nil || !info.Available || info.Containers != 2 || info.Images != 4 {
		t.Fatalf("docker info: %+v %v", info, err)
	}
	list, err := m.containers(context.Background())
	if err != nil || len(list) != 1 || list[0].Name != "web" {
		t.Fatalf("containers: %+v %v", list, err)
	}
	services, err := m.services(context.Background())
	if err != nil || len(services) != 2 || !services[0].Allowed || services[1].Allowed || services[0].Enabled != "enabled" || services[0].State != "active" {
		t.Fatalf("services: %+v %v", services, err)
	}
}
func TestFilesBoundedTraversalSecretAndOverwriteProtection(t *testing.T) {
	if runtime.GOOS == "linux" && os.Geteuid() != 0 {
		t.Skip("protected-root file fixture requires root")
	}
	m := fixture(t)
	ctx := context.Background()
	request := shared.ManagementRequest{Action: "file.upload", Path: "example.txt", Content: base64.StdEncoding.EncodeToString([]byte("hello 云简"))}
	if _, err := m.Execute(ctx, request); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Execute(ctx, request); err == nil {
		t.Fatal("existing file overwritten")
	}
	value, err := m.Execute(ctx, shared.ManagementRequest{Action: "file.read", Path: "example.txt"})
	if err != nil {
		t.Fatal(err)
	}
	data, _ := base64.StdEncoding.DecodeString(value.(map[string]any)["content"].(string))
	if string(data) != "hello 云简" {
		t.Fatal("file content mismatch")
	}
	for _, path := range []string{"../outside.txt", "/etc/passwd", "a/../../outside.txt", ".env", "agent-token", "credentials.json", "key.pem", "cloudease.db", "a\\..\\outside.txt"} {
		if _, err := m.Execute(ctx, shared.ManagementRequest{Action: "file.read", Path: path}); err == nil {
			t.Fatalf("unsafe path accepted: %s", path)
		}
	}
	request.Path = "script.sh"
	if _, err := m.Execute(ctx, request); err == nil {
		t.Fatal("executable upload accepted")
	}
	request.Path = "large.txt"
	request.Content = base64.StdEncoding.EncodeToString(make([]byte, shared.MaxFileBytes+1))
	if _, err := m.Execute(ctx, request); err == nil {
		t.Fatal("oversized upload accepted")
	}
	os.WriteFile(filepath.Join(m.Roots[0], ".env"), []byte("secret"), 0600)
	list, err := m.Execute(ctx, shared.ManagementRequest{Action: "file.list", Path: "."})
	if err != nil {
		t.Fatal(err)
	}
	if len(list.([]shared.FileEntry)) != 1 {
		t.Fatalf("hidden files listed: %+v", list)
	}
	outside := filepath.Join(t.TempDir(), "outside.txt")
	os.WriteFile(outside, []byte("outside"), 0600)
	link := filepath.Join(m.Roots[0], "escape.txt")
	if err := os.Symlink(outside, link); err == nil {
		if _, err := m.Execute(ctx, shared.ManagementRequest{Action: "file.read", Path: "escape.txt"}); err == nil {
			t.Fatal("symlink escaped root")
		}
	} else {
		t.Log("symlink creation not supported by this test account; os.Root escape covered by Go platform implementation")
	}
}
func TestForbiddenRootsAndServiceNames(t *testing.T) {
	for _, name := range []string{"sshd", "ssh", "systemd-networkd", "kernel", "cloud-agent", "polkit"} {
		if !deniedService(name) {
			t.Fatalf("service not denied: %s", name)
		}
	}
	if runtime.GOOS == "linux" {
		for _, root := range []string{"/", "/etc", "/root", "/home/a", "/proc/self", "/opt/cloudease-lite", "/var/lib/docker"} {
			if safeRoot(root) {
				t.Fatalf("root accepted: %s", root)
			}
		}
	}
	if runtime.GOOS != "linux" {
		m, err := New(false, "", "")
		if err != nil || m.Enabled {
			t.Fatalf("legacy Windows startup: %v", err)
		}
	}
}

func TestBoundedCommandOutputAndJSONLogRedaction(t *testing.T) {
	out := &boundedOutput{}
	input := []byte(strings.Repeat("x", shared.MaxOutputBytes*2))
	if n, err := out.Write(input); err != nil || n != len(input) || len(out.data) != shared.MaxOutputBytes {
		t.Fatal("output bound failed")
	}
	if n, err := out.Write(input); err != nil || n != len(input) || len(out.data) != shared.MaxOutputBytes {
		t.Fatal("output pipe stopped draining")
	}
	value := shared.RedactOutput(`{"password":"must-be-hidden","message":"ok"}`)
	if strings.Contains(value, "must-be-hidden") {
		t.Fatal("JSON-like log secret leaked")
	}
}

func TestNestedDirectoriesAndSecretSymlinkAliases(t *testing.T) {
	if runtime.GOOS == "linux" && os.Geteuid() != 0 {
		t.Skip("protected-root file fixture requires root")
	}
	m := fixture(t)
	ctx := context.Background()
	if err := os.Mkdir(filepath.Join(m.Roots[0], "notes"), 0700); err != nil {
		t.Fatal(err)
	}
	req := shared.ManagementRequest{Action: "file.upload", Path: "notes/readme.txt", Content: base64.StdEncoding.EncodeToString([]byte("nested"))}
	if _, err := m.Execute(ctx, req); err != nil {
		t.Fatal(err)
	}
	value, err := m.Execute(ctx, shared.ManagementRequest{Action: "file.list", Path: "notes"})
	if err != nil || len(value.([]shared.FileEntry)) != 1 {
		t.Fatalf("nested directory: %v %v", value, err)
	}
	if _, err := m.Execute(ctx, shared.ManagementRequest{Action: "file.read", Path: "notes/readme.txt"}); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(m.Roots[0], ".env"), []byte("not-for-viewers"), 0600)
	if err := os.Symlink(".env", filepath.Join(m.Roots[0], "alias.txt")); err != nil {
		t.Log("symlink fixture unavailable:", err)
		return
	}
	if _, err := m.Execute(ctx, shared.ManagementRequest{Action: "file.read", Path: "alias.txt"}); err == nil {
		t.Fatal("sensitive filename bypassed by alias")
	}
	os.Mkdir(filepath.Join(m.Roots[0], ".hidden"), 0700)
	os.WriteFile(filepath.Join(m.Roots[0], ".hidden", "readme.txt"), []byte("private"), 0600)
	if err := os.Symlink(".hidden", filepath.Join(m.Roots[0], "visible")); err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"file.list", "file.read", "file.upload"} {
		path := "visible/readme.txt"
		if action == "file.list" {
			path = "visible"
		}
		req := shared.ManagementRequest{Action: action, Path: path, Content: base64.StdEncoding.EncodeToString([]byte("new"))}
		if action != "file.upload" {
			req.Content = ""
		}
		if _, err := m.Execute(ctx, req); err == nil {
			t.Fatalf("directory alias accepted for %s", action)
		}
	}
}
