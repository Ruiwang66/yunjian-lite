// Package management executes fixed operations against the local Linux host.
package management

import (
	"cloudease-lite/shared"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

type Runner func(context.Context, string, ...string) ([]byte, error)
type Manager struct {
	Enabled    bool
	Roots      []string
	Services   map[string]bool
	Run        Runner
	privileged bool
	platform   string
}

func New(enabled bool, roots, allowed string) (*Manager, error) {
	m := &Manager{Enabled: enabled, Services: map[string]bool{}, Run: runCommand, privileged: privileged(), platform: runtime.GOOS}
	if runtime.GOOS != "linux" {
		m.Enabled = false
		return m, nil
	}
	if roots == "" {
		roots = "/opt/cloud-ease"
	}
	for _, root := range strings.Split(roots, ",") {
		root = filepath.Clean(strings.TrimSpace(root))
		if !safeRoot(root) {
			return nil, errors.New("文件根目录必须是独立绝对目录，不能位于系统、账户或客户端配置目录")
		}
		m.Roots = append(m.Roots, root)
	}
	if len(m.Roots) > 8 {
		return nil, errors.New("最多允许 8 个文件目录")
	}
	if allowed == "" {
		allowed = "nginx,mysql,docker,php-fpm,redis"
	}
	for _, unit := range strings.Split(allowed, ",") {
		unit = strings.TrimSuffix(strings.TrimSpace(unit), ".service")
		if !unitPattern.MatchString(unit) || deniedService(unit) {
			return nil, errors.New("服务白名单包含禁止或无效的服务")
		}
		m.Services[unit+".service"] = true
	}
	if len(m.Services) > 32 {
		return nil, errors.New("最多允许 32 个服务")
	}
	return m, nil
}

var unitPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.@-]{0,127}$`)
var containerPattern = regexp.MustCompile(`^[a-f0-9]{12,64}$`)

func deniedService(name string) bool {
	for _, bad := range []string{"ssh", "sshd", "systemd", "kernel", "kmod", "udev", "modprobe", "modules-load", "sysctl", "init", "reboot", "shutdown", "poweroff", "halt", "cloud-agent", "cloudease", "getty", "dbus", "polkit"} {
		if strings.HasPrefix(strings.ToLower(name), bad) {
			return true
		}
	}
	return false
}
func safeRoot(root string) bool {
	if !filepath.IsAbs(root) || root == string(filepath.Separator) {
		return false
	}
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	for _, deny := range []string{"/etc", "/root", "/home", "/proc", "/sys", "/dev", "/boot", "/bin", "/sbin", "/usr", "/lib", "/lib64", "/run", "/opt/cloudease-lite", "/var/lib/docker", "/var/lib/cloudease-lite"} {
		// Reject both descendants and a root which contains any forbidden tree.
		if root == deny || strings.HasPrefix(root, deny+"/") || strings.HasPrefix(deny, root+"/") {
			return false
		}
	}
	return true
}
func (m *Manager) Snapshot(ctx context.Context) shared.ManagementInfo {
	info := shared.ManagementInfo{Version: shared.Version, Enabled: m.Enabled && m.platform == "linux", Privileged: m.privileged, FileRoots: []string{}, Services: []shared.Service{}}
	if m.platform != "linux" {
		return info
	}
	info.Docker, _ = m.dockerInfo(ctx)
	services, serviceErr := m.services(ctx)
	info.Services = services
	info.DockerWrite = info.Enabled && info.Docker.Available
	info.ServiceWrite = info.Enabled && m.privileged && serviceErr == nil
	if info.Enabled {
		for _, root := range m.Roots {
			if fileWritable(root) {
				info.FileWrite = true
			}
		}
	}
	if info.Enabled {
		info.FileRoots = append(info.FileRoots, m.Roots...)
	}
	return info
}
func (m *Manager) Execute(ctx context.Context, req shared.ManagementRequest) (any, error) {
	if !m.Enabled || m.platform != "linux" {
		return nil, errors.New("此客户端未启用 Linux 运维管理")
	}
	if !shared.KnownAction(req.Action) || req.Root < 0 || req.Root >= len(m.Roots) {
		return nil, errors.New("操作或目录编号不允许")
	}
	if !strings.HasPrefix(req.Action, "file.") && (req.Path != "" || req.Content != "" || req.Root != 0) {
		return nil, errors.New("此操作不允许额外文件参数")
	}
	if (strings.HasPrefix(req.Action, "command.") || req.Action == "docker.list" || req.Action == "docker.info" || req.Action == "service.list") && req.Target != "" {
		return nil, errors.New("此操作不允许目标参数")
	}
	if req.Action != "file.upload" && req.Content != "" {
		return nil, errors.New("此操作不允许上传内容")
	}
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	switch req.Action {
	case "docker.info":
		return m.dockerInfo(ctx)
	case "docker.list":
		return m.containers(ctx)
	case "docker.logs", "docker.start", "docker.stop", "docker.restart":
		if !containerPattern.MatchString(req.Target) {
			return nil, errors.New("容器编号无效")
		}
		action := strings.TrimPrefix(req.Action, "docker.")
		args := []string{action, req.Target}
		if action == "logs" {
			args = []string{"logs", "--tail", "100", "--timestamps", req.Target}
		}
		if action == "stop" || action == "restart" {
			args = []string{action, "--time", "5", req.Target}
		}
		out, err := m.Run(ctx, "docker", args...)
		if err != nil {
			return nil, errors.New("Docker 操作失败，请检查客户端权限和容器状态")
		}
		return map[string]any{"output": shared.RedactOutput(string(out))}, nil
	case "service.list":
		return m.services(ctx)
	case "service.start", "service.stop", "service.restart":
		target := strings.TrimSuffix(req.Target, ".service")
		if !m.privileged || deniedService(target) || !m.Services[target+".service"] {
			return nil, errors.New("服务不在白名单或客户端权限不足")
		}
		_, err := m.Run(ctx, "systemctl", strings.TrimPrefix(req.Action, "service."), target+".service")
		if err != nil {
			return nil, errors.New("服务操作失败，请检查 systemd 日志")
		}
		return map[string]any{"output": "服务操作完成"}, nil
	case "command.disk", "command.memory", "command.network", "command.journal":
		name, args := "df", []string{"-h"}
		switch req.Action {
		case "command.memory":
			name, args = "free", []string{"-m"}
		case "command.network":
			name, args = "ip", []string{"addr"}
		case "command.journal":
			name, args = "journalctl", []string{"-n", "100", "--no-pager", "--output", "short-iso"}
		}
		out, err := m.Run(ctx, name, args...)
		if err != nil {
			return nil, errors.New("预定义命令失败，请检查客户端权限或工具是否可用")
		}
		return map[string]any{"output": shared.RedactOutput(string(out))}, nil
	default:
		return m.files(req)
	}
}
func (m *Manager) dockerInfo(ctx context.Context) (shared.DockerInfo, error) {
	var info shared.DockerInfo
	out, err := m.Run(ctx, "docker", "info", "--format", `{"ServerVersion":{{json .ServerVersion}},"Containers":{{.Containers}},"Images":{{.Images}}}`)
	if err != nil {
		return info, err
	}
	var raw struct {
		ServerVersion      string
		Containers, Images int
	}
	if err = json.Unmarshal(out, &raw); err != nil {
		return info, err
	}
	return shared.DockerInfo{Available: true, Version: raw.ServerVersion, Containers: raw.Containers, Images: raw.Images}, nil
}
func (m *Manager) containers(ctx context.Context) ([]shared.Container, error) {
	out, err := m.Run(ctx, "docker", "ps", "--all", "--no-trunc", "--format", "{{json .}}")
	if err != nil {
		return nil, errors.New("Docker 不可用或权限不足")
	}
	result := []shared.Container{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		var raw struct{ ID, Names, State, Ports, CreatedAt string }
		if json.Unmarshal([]byte(line), &raw) != nil {
			return nil, errors.New("容器信息被截断或格式无效")
		}
		result = append(result, shared.Container{ID: raw.ID, Name: raw.Names, State: raw.State, Ports: raw.Ports, CreatedAt: raw.CreatedAt})
		if len(result) >= 200 {
			break
		}
	}
	return result, nil
}
func (m *Manager) services(ctx context.Context) ([]shared.Service, error) {
	enabled, err := m.Run(ctx, "systemctl", "list-unit-files", "--type=service", "--no-legend", "--no-pager")
	if err != nil {
		return nil, errors.New("systemd 不可用")
	}
	states, err := m.Run(ctx, "systemctl", "list-units", "--type=service", "--all", "--plain", "--no-legend", "--no-pager")
	if err != nil {
		return nil, errors.New("systemd 不可用")
	}
	items := map[string]shared.Service{}
	for _, line := range strings.Split(string(enabled), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || !strings.HasSuffix(fields[0], ".service") {
			continue
		}
		items[fields[0]] = shared.Service{Name: fields[0], State: "inactive", Enabled: fields[1], Allowed: m.Services[fields[0]]}
	}
	for _, line := range strings.Split(string(states), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 || !strings.HasSuffix(fields[0], ".service") {
			continue
		}
		item := items[fields[0]]
		item.Name, item.State, item.Allowed = fields[0], fields[2], m.Services[fields[0]]
		if item.Enabled == "" {
			item.Enabled = "unknown"
		}
		items[fields[0]] = item
	}
	result := []shared.Service{}
	for _, item := range items {
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Allowed != result[j].Allowed {
			return result[i].Allowed
		}
		return result[i].Name < result[j].Name
	})
	if len(result) > 200 {
		result = result[:200]
	}
	return result, nil
}
func safeFilePath(name string) bool {
	if name == "." {
		return true
	}
	if !fs.ValidPath(name) || len(name) > 2048 || strings.Count(name, "/") >= 32 || strings.ContainsAny(name, "\\:\x00") {
		return false
	}
	for _, part := range strings.Split(name, "/") {
		if sensitiveName(part) {
			return false
		}
	}
	return true
}
func sensitiveName(name string) bool {
	name = strings.ToLower(name)
	if strings.HasPrefix(name, ".") {
		return true
	}
	for _, v := range []string{"password", "passwd", "token", "secret", "credential", "private", "id_rsa", "id_ed25519", "agent.env", "cloudease"} {
		if strings.Contains(name, v) {
			return true
		}
	}
	for _, v := range []string{".pem", ".key", ".pfx", ".p12", ".db", ".sqlite", ".sqlite3", ".db-wal", ".db-shm", ".db-journal", ".sqlite-wal", ".sqlite-shm", ".sqlite-journal"} {
		if strings.HasSuffix(name, v) {
			return true
		}
	}
	return false
}
func (m *Manager) files(req shared.ManagementRequest) (any, error) {
	if !safeFilePath(req.Path) {
		return nil, errors.New("文件路径或文件类型不允许")
	}
	rootPath := m.Roots[req.Root]
	resolved, err := filepath.EvalSymlinks(rootPath)
	if err != nil || resolved != rootPath || !safeRoot(resolved) {
		return nil, errors.New("允许目录不存在或包含符号链接")
	}
	if err := secureRoot(rootPath); err != nil {
		return nil, errors.New("允许目录必须由 root 所有且父目录不能允许其他用户写入")
	}
	initial, err := os.Lstat(rootPath)
	if err != nil || !initial.IsDir() || initial.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("允许目录无效")
	}
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return nil, errors.New("允许目录不可访问")
	}
	defer root.Close()
	anchor, err := root.Open(".")
	if err != nil {
		return nil, errors.New("无法验证目录")
	}
	actual, err := anchor.Stat()
	anchor.Close()
	if err != nil || !os.SameFile(initial, actual) {
		return nil, errors.New("目录在访问期间发生变化")
	}
	parent, leaf, cleanup, err := pinParent(root, req.Path)
	if err != nil {
		return nil, errors.New("文件父目录不可访问或包含符号链接")
	}
	defer cleanup()
	if req.Action == "file.upload" {
		data, err := base64.StdEncoding.DecodeString(req.Content)
		if err != nil || len(data) > shared.MaxFileBytes || !utf8.Valid(data) {
			return nil, errors.New("仅支持最多 32 KiB 的 UTF-8 文本上传")
		}
		switch strings.ToLower(filepath.Ext(req.Path)) {
		case ".txt", ".md", ".csv", ".json", ".yaml", ".yml", ".log":
		default:
			return nil, errors.New("上传文件扩展名不允许")
		}
		// Exclusive creation never overwrites existing files, hardlinks, symlinks or executables.
		f, err := parent.OpenFile(leaf, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return nil, errors.New("上传失败：目录不可写或同名文件已存在")
		}
		_, err = f.Write(data)
		closeErr := f.Close()
		if err != nil || closeErr != nil {
			return nil, errors.New("文件写入失败")
		}
		return map[string]any{"size": len(data)}, nil
	}
	before, err := parent.Lstat(leaf)
	if err != nil || before.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("文件不存在或包含符号链接")
	}
	directory := parent
	if req.Action == "file.list" && leaf != "." {
		directory, err = pinDirectory(parent, leaf)
		if err != nil {
			return nil, errors.New("目录不可访问或包含符号链接")
		}
		defer directory.Close()
	}
	var f *os.File
	if req.Action == "file.list" {
		f, err = directory.Open(".")
	} else {
		f, err = openNonblocking(parent, leaf)
	}
	if err != nil {
		return nil, errors.New("文件不可访问或路径越界")
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !os.SameFile(before, info) {
		return nil, errors.New("文件状态不可读取或访问期间发生变化")
	}
	if req.Action == "file.list" {
		if !info.IsDir() {
			return nil, errors.New("路径不是目录")
		}
		entries, err := f.ReadDir(1001)
		if err != nil && err != io.EOF {
			return nil, errors.New("目录不可读取")
		}
		if len(entries) > 1000 {
			return nil, errors.New("目录超过 1000 项，请选择更小的子目录")
		}
		result := []shared.FileEntry{}
		for _, e := range entries {
			if sensitiveName(e.Name()) || e.Type()&os.ModeSymlink != 0 {
				continue
			}
			stat, err := directory.Lstat(e.Name())
			if err != nil || stat.Mode()&os.ModeSymlink != 0 || (!stat.IsDir() && !regularSingleLink(stat)) {
				continue
			}
			result = append(result, shared.FileEntry{Name: e.Name(), Directory: stat.IsDir(), Size: stat.Size(), ModifiedAt: stat.ModTime().Unix()})
		}
		sort.Slice(result, func(i, j int) bool {
			if result[i].Directory != result[j].Directory {
				return result[i].Directory
			}
			return result[i].Name < result[j].Name
		})
		encoded, _ := json.Marshal(result)
		if len(encoded) > 48000 {
			return nil, errors.New("目录数据过大，请选择更小的子目录")
		}
		return result, nil
	}
	if !regularSingleLink(info) || info.Size() > shared.MaxFileBytes {
		return nil, errors.New("仅允许读取普通文件，最多 32 KiB")
	}
	data, err := io.ReadAll(io.LimitReader(f, shared.MaxFileBytes+1))
	if err != nil || len(data) > shared.MaxFileBytes {
		return nil, errors.New("文件读取失败或文件过大")
	}
	if req.Action == "file.read" {
		if !utf8.Valid(data) {
			return nil, errors.New("此文件不是 UTF-8 文本，请下载查看")
		}
		// Raw text is transported as base64 too, keeping the JSON response bounded.
	}
	return map[string]any{"content": base64.StdEncoding.EncodeToString(data), "size": len(data), "name": filepath.Base(req.Path)}, nil
}
func (m *Manager) String() string {
	return fmt.Sprintf("management=%t privileged=%t", m.Enabled, m.privileged)
}
