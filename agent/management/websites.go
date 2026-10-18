package management

import (
	"cloudease-lite/shared"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

// Host-only configuration is never accepted from the control plane.
type SiteConfig struct {
	shared.WebsiteSpec
	SQLitePath        string `json:"sqlite_path,omitempty"`
	MySQLDatabase     string `json:"mysql_database,omitempty"`
	MySQLDefaultsFile string `json:"mysql_defaults_file,omitempty"`
}
type WebsiteConfig struct {
	AvailableDir string       `json:"nginx_available_dir"`
	EnabledDir   string       `json:"nginx_enabled_dir"`
	BackupDir    string       `json:"backup_dir"`
	Sites        []SiteConfig `json:"sites"`
}
type websiteManager struct {
	config    WebsiteConfig
	mu        sync.Mutex
	probeMu   sync.Mutex
	nextProbe int
	checks    map[string]shared.WebsiteCheck
}

func (m *Manager) ConfigureWebsites(name string) error {
	if name == "" {
		return nil
	}
	if !m.Enabled || m.platform != "linux" || !m.privileged {
		return errors.New("网站管理需要已启用运维的 Linux root 客户端")
	}
	if err := secureRoot(filepath.Dir(name)); err != nil {
		return errors.New("网站配置父目录必须为 root 所有且不能被其他用户写入")
	}
	before, err := os.Lstat(name)
	if err != nil || !regularSingleLink(before) || !rootOwned(before) || before.Mode().Perm()&0077 != 0 || before.Size() > 32768 {
		return errors.New("网站配置必须是 root 专用的普通文件（0600，最多 32 KiB）")
	}
	f, err := os.Open(name)
	if err != nil {
		return err
	}
	defer f.Close()
	actual, err := f.Stat()
	if err != nil || !os.SameFile(before, actual) {
		return errors.New("网站配置已变化")
	}
	var cfg WebsiteConfig
	decoder := json.NewDecoder(io.LimitReader(f, 32769))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&cfg) != nil || decoder.Decode(new(any)) != io.EOF {
		return errors.New("网站配置 JSON 无效")
	}
	if err = validateWebsiteConfig(cfg); err != nil {
		return err
	}
	m.websites = &websiteManager{config: cfg}
	return nil
}
func validateWebsiteConfig(cfg WebsiteConfig) error {
	if len(cfg.Sites) == 0 || len(cfg.Sites) > 20 {
		return errors.New("网站白名单允许 1–20 个站点")
	}
	for _, dir := range []string{cfg.AvailableDir, cfg.EnabledDir, cfg.BackupDir} {
		if !filepath.IsAbs(dir) || filepath.Clean(dir) != dir {
			return errors.New("网站配置目录必须为规范绝对路径")
		}
		if secureRoot(dir) != nil {
			return errors.New("网站配置目录必须存在且为 root 所有，禁止可写父目录和符号链接")
		}
	}
	if cfg.AvailableDir == cfg.EnabledDir || !safeRoot(cfg.BackupDir) {
		return errors.New("网站配置目录或备份目录不安全")
	}
	if info, err := os.Stat(cfg.BackupDir); err != nil || info.Mode().Perm()&0077 != 0 && mPathLinux() {
		return errors.New("备份目录必须为 root 专用 0700")
	}
	if mPathLinux() && (cfg.AvailableDir != "/etc/nginx/sites-available" || cfg.EnabledDir != "/etc/nginx/sites-enabled") {
		return errors.New("站点启停仅支持 /etc/nginx/sites-available 与 sites-enabled 布局")
	}
	seen := map[string]bool{}
	configs := map[string]bool{}
	for _, site := range cfg.Sites {
		if site.SQLitePath != "" && site.MySQLDatabase != "" {
			return errors.New("每个网站只允许选择一种数据库备份类型")
		}
		if !shared.ValidWebsite(site.WebsiteSpec) || !safeRoot(site.RootPath) || secureRoot(site.RootPath) != nil || seen[site.Domain] || configs[site.NginxConfig] {
			return errors.New("网站白名单包含无效、重复或不安全的站点")
		}
		seen[site.Domain] = true
		configs[site.NginxConfig] = true
		for _, dir := range []string{cfg.AvailableDir, cfg.EnabledDir, cfg.BackupDir} {
			if dir == site.RootPath || strings.HasPrefix(dir, site.RootPath+string(filepath.Separator)) {
				return errors.New("配置和备份不能位于网站目录内")
			}
		}
		if site.SQLitePath != "" && (!filepath.IsAbs(site.SQLitePath) || filepath.Clean(site.SQLitePath) != site.SQLitePath || !safeRoot(filepath.Dir(site.SQLitePath)) || secureRoot(filepath.Dir(site.SQLitePath)) != nil) {
			return errors.New("SQLite 路径不安全")
		}
		if site.MySQLDatabase != "" && (!regexp.MustCompile(`^[A-Za-z0-9_]{1,64}$`).MatchString(site.MySQLDatabase) || site.MySQLDefaultsFile == "" || !filepath.IsAbs(site.MySQLDefaultsFile) || secureRoot(filepath.Dir(site.MySQLDefaultsFile)) != nil) {
			return errors.New("MySQL 本地备份配置无效")
		}
	}
	return nil
}
func (m *Manager) WebsiteSnapshot(ctx context.Context) *shared.WebsiteInventory {
	if m.websites == nil {
		return nil
	}
	inventory := &shared.WebsiteInventory{Enabled: true, Nginx: m.nginxStatus(ctx), PHP: m.phpInspect(ctx), Sites: []shared.WebsiteSpec{}, Checks: []shared.WebsiteCheck{}}
	w := m.websites
	w.probeMu.Lock()
	start := w.nextProbe
	w.probeMu.Unlock()
	for _, site := range w.config.Sites {
		inventory.Sites = append(inventory.Sites, site.WebsiteSpec)
	}
	for n := 0; n < len(w.config.Sites) && ctx.Err() == nil; n++ {
		i := (start + n) % len(w.config.Sites)
		site := w.config.Sites[i]
		check := probeWebsite(ctx, site.Domain)
		check.Warnings = append(check.Warnings, siteWarnings(site.RootPath)...)
		w.probeMu.Lock()
		w.nextProbe = (i + 1) % len(w.config.Sites)
		if w.checks == nil {
			w.checks = map[string]shared.WebsiteCheck{}
		}
		if ctx.Err() == nil {
			w.checks[site.Domain] = check
		}
		w.probeMu.Unlock()
	}
	w.probeMu.Lock()
	for _, site := range w.config.Sites {
		if check, ok := w.checks[site.Domain]; ok {
			inventory.Checks = append(inventory.Checks, check)
		}
	}
	w.probeMu.Unlock()
	return inventory
}
func (m *Manager) websiteOperation(ctx context.Context, req shared.ManagementRequest) (any, error) {
	if m.websites == nil {
		return nil, errors.New("客户端未配置 v0.3 网站白名单")
	}
	m.websites.mu.Lock()
	defer m.websites.mu.Unlock()
	if req.Action == "nginx.status" {
		if req.Target != "" || req.Website != nil {
			return nil, errors.New("环境读取不允许站点参数")
		}
		return m.nginxStatus(ctx), nil
	}
	if req.Action == "php.inspect" {
		if req.Target != "" || req.Website != nil {
			return nil, errors.New("环境读取不允许站点参数")
		}
		return m.phpInspect(ctx), nil
	}
	if req.Action == "nginx.sites" {
		if req.Target != "" || req.Website != nil {
			return nil, errors.New("环境读取不允许站点参数")
		}
		sites := []shared.SiteResult{}
		for _, site := range m.websites.config.Sites {
			sites = append(sites, shared.SiteResult{Domain: site.Domain, Enabled: m.siteEnabled(site)})
		}
		return sites, nil
	}
	if req.Website == nil || !shared.ValidWebsite(*req.Website) || req.Target != req.Website.Domain {
		return nil, errors.New("网站参数无效")
	}
	var selected *SiteConfig
	for i := range m.websites.config.Sites {
		s := &m.websites.config.Sites[i]
		if s.Domain == req.Target && s.RootPath == req.Website.RootPath && s.NginxConfig == req.Website.NginxConfig {
			selected = s
			break
		}
	}
	if selected == nil {
		return nil, errors.New("网站不在客户端本地白名单")
	}
	switch req.Action {
	case "website.check", "domain.check", "ssl.check":
		check := probeWebsite(ctx, selected.Domain)
		check.Warnings = siteWarnings(selected.RootPath)
		return check, nil
	case "nginx.read":
		return m.readSite(*selected)
	case "nginx.enable", "nginx.disable":
		return m.changeSite(ctx, *selected, req.Action == "nginx.enable")
	case "backup.files", "backup.database":
		return m.backupSite(ctx, *selected, strings.TrimPrefix(req.Action, "backup."))
	}
	return nil, errors.New("网站操作不允许")
}
func (m *Manager) nginxStatus(ctx context.Context) shared.NginxInfo {
	result := shared.NginxInfo{State: "不可用"}
	data, err := m.Run(ctx, "nginx", "-v")
	if err != nil {
		return result
	}
	result.Available = true
	result.Version = strings.TrimSpace(string(data))
	state, err := m.Run(ctx, "systemctl", "is-active", "nginx.service")
	result.State = strings.TrimSpace(string(state))
	if err != nil && result.State == "" {
		result.State = "unknown"
	}
	return result
}
func (m *Manager) phpInspect(ctx context.Context) shared.PHPInfo {
	result := shared.PHPInfo{FPMState: "未检测到", Extensions: []string{}, Warnings: []string{}}
	output, err := m.Run(ctx, "php", "-n", "-v")
	if err != nil {
		return result
	}
	result.Available = true
	result.Version = strings.Split(strings.TrimSpace(string(output)), "\n")[0]
	output, err = m.Run(ctx, "php", "-m")
	if err == nil {
		for _, name := range []string{"curl", "gd", "mysqli", "PDO", "openssl"} {
			for _, line := range strings.Split(string(output), "\n") {
				if strings.EqualFold(strings.TrimSpace(line), name) {
					result.Extensions = append(result.Extensions, strings.ToLower(name))
					break
				}
			}
		}
	}
	output, err = m.Run(ctx, "systemctl", "list-units", "--type=service", "--all", "--plain", "--no-legend", "--no-pager")
	if err == nil {
		for _, line := range strings.Split(string(output), "\n") {
			fields := strings.Fields(line)
			if len(fields) >= 4 && regexp.MustCompile(`^php(?:[0-9]+\.[0-9]+)?-fpm\.service$`).MatchString(fields[0]) {
				result.FPMState = fields[0] + ": " + fields[2]
				if fields[2] == "active" {
					break
				}
			}
		}
	}
	output, err = m.Run(ctx, "php", "-i")
	if err == nil {
		for _, line := range strings.Split(string(output), "\n") {
			if strings.HasPrefix(line, "expose_php => On") {
				result.Warnings = append(result.Warnings, "expose_php 已开启")
			}
			if strings.HasPrefix(line, "display_errors => On") {
				result.Warnings = append(result.Warnings, "display_errors 已开启")
			}
			if strings.HasPrefix(line, "disable_functions => no value") {
				result.Warnings = append(result.Warnings, "未配置 disable_functions")
			}
		}
	}
	return result
}
func siteWarnings(root string) []string {
	result := []string{}
	if secureRoot(root) != nil {
		return []string{"网站目录或父目录权限不安全"}
	}
	info, err := os.Stat(root)
	if err == nil && info.Mode().Perm()&0002 != 0 {
		result = append(result, "网站目录允许任意用户写入")
	}
	handle, err := os.OpenRoot(root)
	if err != nil {
		return append(result, "网站目录不可读取")
	}
	defer handle.Close()
	for _, name := range []string{".env", ".git", "backup", "backups"} {
		if _, err := handle.Lstat(name); err == nil {
			result = append(result, "网站根目录存在敏感项："+name+"，请配置 HTTP 访问拒绝规则")
		}
	}
	return result
}
