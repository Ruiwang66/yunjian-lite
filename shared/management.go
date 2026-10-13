package shared

import (
	"encoding/json"
	"regexp"
	"strings"
)

const Version = "0.2.0"
const MaxFileBytes = 32 * 1024
const MaxOutputBytes = 16 * 1024

type SystemInfo struct {
	CPUModel       string   `json:"cpu_model"`
	KernelVersion  string   `json:"kernel_version"`
	Virtualization string   `json:"virtualization"`
	PrivateIPs     []string `json:"private_ips"`
	PublicIPs      []string `json:"public_ips"`
}
type DockerInfo struct {
	Available  bool   `json:"available"`
	Version    string `json:"version"`
	Containers int    `json:"containers"`
	Images     int    `json:"images"`
}
type Container struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	State     string `json:"state"`
	Ports     string `json:"ports"`
	CreatedAt string `json:"created_at"`
}
type Service struct {
	Name    string `json:"name"`
	State   string `json:"state"`
	Enabled string `json:"enabled"`
	Allowed bool   `json:"allowed"`
}
type ManagementInfo struct {
	Version      string     `json:"version"`
	Enabled      bool       `json:"enabled"`
	Privileged   bool       `json:"privileged"`
	DockerWrite  bool       `json:"docker_write"`
	ServiceWrite bool       `json:"service_write"`
	FileWrite    bool       `json:"file_write"`
	FileRoots    []string   `json:"file_roots"`
	Docker       DockerInfo `json:"docker"`
	Services     []Service  `json:"services"`
}

// Only typed, fixed actions cross the control plane; there is no shell or argv field.
type ManagementRequest struct {
	Action  string `json:"action"`
	Target  string `json:"target,omitempty"`
	Root    int    `json:"root,omitempty"`
	Path    string `json:"path,omitempty"`
	Content string `json:"content,omitempty"` // base64, bounded to 32 KiB after decoding
}
type ManagementTask struct {
	ID        string            `json:"id"`
	Lease     string            `json:"lease"`
	ExpiresAt int64             `json:"expires_at"`
	Request   ManagementRequest `json:"request"`
}
type ManagementResult struct {
	ID      string          `json:"id"`
	Lease   string          `json:"lease"`
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data,omitempty"`
	Error   string          `json:"error,omitempty"`
}
type FileEntry struct {
	Name       string `json:"name"`
	Directory  bool   `json:"directory"`
	Size       int64  `json:"size"`
	ModifiedAt int64  `json:"modified_at"`
}

func MutatingAction(action string) bool {
	switch action {
	case "docker.start", "docker.stop", "docker.restart", "service.start", "service.stop", "service.restart", "file.upload":
		return true
	}
	return false
}
func KnownAction(action string) bool {
	switch action {
	case "docker.info", "docker.list", "docker.logs", "docker.start", "docker.stop", "docker.restart", "service.list", "service.start", "service.stop", "service.restart", "file.list", "file.read", "file.download", "file.upload", "command.disk", "command.memory", "command.network", "command.journal":
		return true
	}
	return false
}

var sensitiveOutput = regexp.MustCompile(`(?im)(\b(?:password|passwd|token|secret|api[_-]?key|authorization)\b["']?\s*[=:]\s*)([^\r\n]+)`)
var bearerOutput = regexp.MustCompile(`(?i)Bearer\s+[A-Za-z0-9._-]+`)

func RedactOutput(value string) string {
	value = bearerOutput.ReplaceAllString(value, "Bearer [已隐藏]")
	return sensitiveOutput.ReplaceAllString(value, "${1}[已隐藏]")
}
func SafeLabel(s string) string {
	s = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return -1
		}
		return r
	}, s)
	if len(s) > 240 {
		s = s[:240]
	}
	return s
}
