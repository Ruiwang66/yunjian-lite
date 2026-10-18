package management

import (
	"cloudease-lite/shared"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

type boundedOutput struct {
	mu   sync.Mutex
	data []byte
}

func (b *boundedOutput) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := len(p)
	remaining := shared.MaxOutputBytes - len(b.data)
	if remaining > 0 {
		if len(p) > remaining {
			p = p[:remaining]
		}
		b.data = append(b.data, p...)
	}
	return n, nil // Keep draining the pipe after the limit to avoid deadlock.
}
func runCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	allowed := map[string]bool{"docker": true, "systemctl": true, "df": true, "free": true, "ip": true, "journalctl": true, "nginx": true, "php": true}
	if !allowed[name] {
		return nil, errors.New("executable denied")
	}
	var binary string
	for _, dir := range []string{"/usr/bin", "/bin", "/usr/sbin", "/sbin"} {
		path := filepath.Join(dir, name)
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0111 != 0 {
			binary = path
			break
		}
	}
	if binary == "" {
		return nil, errors.New("executable unavailable")
	}
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Env = []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "LANG=C", "LC_ALL=C", "HOME=/nonexistent", "DOCKER_CONFIG=/nonexistent", "DOCKER_HOST=unix:///var/run/docker.sock", "SYSTEMD_PAGER=cat"}
	cmd.WaitDelay = 2 * time.Second
	output := &boundedOutput{}
	cmd.Stdout, cmd.Stderr = output, output
	err := cmd.Run()
	return output.data, err
}
