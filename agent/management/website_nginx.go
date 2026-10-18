package management

import (
	"cloudease-lite/shared"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

func protectedOpen(dir, name string, max int64) (*os.File, error) {
	if secureRoot(dir) != nil {
		return nil, errors.New("配置目录权限不安全")
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	info, err := root.Lstat(name)
	if err != nil || !regularSingleLink(info) || !rootOwned(info) || mPathLinux() && info.Mode().Perm()&0022 != 0 || info.Size() > max {
		return nil, errors.New("配置必须为 root 所有的有界普通文件，拒绝链接及其他用户可写文件")
	}
	file, err := openNonblocking(root, name)
	if err != nil {
		return nil, err
	}
	actual, err := file.Stat()
	if err != nil || !os.SameFile(info, actual) {
		file.Close()
		return nil, errors.New("配置文件已变化")
	}
	return file, nil
}
func protectedRead(dir, name string, max int64) ([]byte, error) {
	file, err := protectedOpen(dir, name, max)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, max+1))
	if len(data) > int(max) {
		return nil, errors.New("配置文件过大")
	}
	return data, err
}
func (m *Manager) siteEnabled(site SiteConfig) bool {
	target, err := os.Readlink(filepath.Join(m.websites.config.EnabledDir, site.NginxConfig))
	if !filepath.IsAbs(target) {
		target = filepath.Join(m.websites.config.EnabledDir, target)
	}
	return err == nil && filepath.Clean(target) == filepath.Join(m.websites.config.AvailableDir, site.NginxConfig)
}
func (m *Manager) readSite(site SiteConfig) (shared.SiteResult, error) {
	data, err := protectedRead(m.websites.config.AvailableDir, site.NginxConfig, 32768)
	if err != nil {
		return shared.SiteResult{}, err
	}
	if !utf8.Valid(data) {
		return shared.SiteResult{}, errors.New("站点配置不是 UTF-8 文本")
	}
	text := shared.RedactOutput(string(data))
	text = regexp.MustCompile(`(?i)(https?://)[^/\s@]+@`).ReplaceAllString(text, "${1}[已隐藏]@")
	text = regexp.MustCompile(`(?im)^\s*(?:fastcgi_param|proxy_set_header|set)\s+\S*(?:password|passwd|secret|token|api[_-]?key)\S*\s+[^;]*;`).ReplaceAllString(text, "# 敏感参数已隐藏")
	text = regexp.MustCompile(`(?im)^\s*proxy_set_header\s+(?:Authorization|Cookie|X-Api-Key)\s+[^;]*;`).ReplaceAllString(text, "# 敏感请求头已隐藏")
	warnings := siteWarnings(site.RootPath)
	if strings.Contains(text, "autoindex on") {
		warnings = append(warnings, "autoindex on 可能暴露目录列表")
	}
	if strings.Contains(text, "ssl_protocols") && (strings.Contains(text, "TLSv1 ") || strings.Contains(text, "TLSv1.1")) {
		warnings = append(warnings, "配置包含旧 TLS 版本")
	}
	if !strings.Contains(text, ".env") || !strings.Contains(text, ".git") {
		warnings = append(warnings, "请核对 .env / .git 的访问拒绝规则（静态提示）")
	}
	return shared.SiteResult{Domain: site.Domain, Config: text, Enabled: m.siteEnabled(site), Warnings: warnings}, nil
}
func (m *Manager) changeSite(ctx context.Context, site SiteConfig, enable bool) (shared.SiteResult, error) {
	if !m.privileged {
		return shared.SiteResult{}, errors.New("Nginx 修改需要 Linux root 权限")
	}
	cfg := m.websites.config
	for _, dir := range []string{cfg.AvailableDir, cfg.EnabledDir, cfg.BackupDir} {
		if secureRoot(dir) != nil {
			return shared.SiteResult{}, errors.New("配置目录权限已变化")
		}
	}
	data, err := protectedRead(cfg.AvailableDir, site.NginxConfig, 32768)
	if err != nil {
		return shared.SiteResult{}, err
	}
	link := filepath.Join(cfg.EnabledDir, site.NginxConfig)
	target := filepath.Join(cfg.AvailableDir, site.NginxConfig)
	originalTarget := target
	before, err := os.Lstat(link)
	wasEnabled := false
	if err == nil {
		value, e := os.Readlink(link)
		resolved := value
		if !filepath.IsAbs(resolved) {
			resolved = filepath.Join(cfg.EnabledDir, resolved)
		}
		if before.Mode()&os.ModeSymlink == 0 || e != nil || filepath.Clean(resolved) != target {
			return shared.SiteResult{}, errors.New("启用目录存在不属于此站点的文件，拒绝覆盖")
		}
		wasEnabled = true
		originalTarget = value
	} else if !os.IsNotExist(err) {
		return shared.SiteResult{}, err
	}
	if wasEnabled == enable {
		return shared.SiteResult{Domain: site.Domain, Enabled: enable, Warnings: []string{}}, nil
	}
	if info, e := os.Stat(cfg.BackupDir); e != nil || mPathLinux() && info.Mode().Perm()&0077 != 0 {
		return shared.SiteResult{}, errors.New("配置备份目录必须为 root 专用 0700")
	}
	if entries, e := os.ReadDir(cfg.BackupDir); e != nil || len(entries) >= 200 {
		return shared.SiteResult{}, errors.New("配置备份目录不可用或已达 200 项限制")
	}
	file, err := os.CreateTemp(cfg.BackupDir, "nginx-"+time.Now().UTC().Format("20060102-150405")+"-*.conf")
	if err != nil {
		return shared.SiteResult{}, errors.New("自动备份失败，未修改站点")
	}
	backup := file.Name()
	if err = file.Chmod(0600); err == nil {
		_, err = file.Write(data)
	}
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil || closeErr != nil {
		os.Remove(backup)
		return shared.SiteResult{}, errors.New("自动备份失败，未修改站点")
	}
	restore := func() error {
		if wasEnabled {
			return os.Symlink(originalTarget, link)
		}
		return os.Remove(link)
	}
	if enable {
		err = os.Symlink(target, link)
	} else {
		err = os.Remove(link)
	}
	if err != nil {
		return shared.SiteResult{}, errors.New("站点切换失败，配置备份已保留")
	}
	_, err = m.Run(ctx, "nginx", "-t")
	if err == nil {
		_, err = m.Run(ctx, "systemctl", "reload", "nginx.service")
	}
	if err != nil {
		if restore() != nil {
			return shared.SiteResult{}, errors.New("验证或重载失败且回滚失败，请立即检查 Nginx；备份已保留")
		}
		rollbackCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, e := m.Run(rollbackCtx, "nginx", "-t"); e == nil {
			m.Run(rollbackCtx, "systemctl", "reload", "nginx.service")
		}
		return shared.SiteResult{}, errors.New("Nginx 验证或重载失败，已恢复原启用状态；配置备份已保留")
	}
	return shared.SiteResult{Domain: site.Domain, Enabled: enable, Backup: filepath.Base(backup), Warnings: []string{}}, nil
}
