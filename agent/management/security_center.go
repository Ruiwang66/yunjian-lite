package management

import (
	sc "cloudease-lite/agent/security"
	"cloudease-lite/shared"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func (m *Manager) ConfigureSecurity(enabled bool, name string) error {
	if !enabled {
		if name != "" {
			return errors.New("security-config 需要显式 --security")
		}
		return nil
	}
	if !m.Enabled || m.platform != "linux" {
		return errors.New("安全采集需要显式启用 Linux --management --security；普通及容器兼容模式不支持")
	}
	c := sc.New(func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return m.Run(ctx, name, args...)
	})
	if name != "" {
		if !filepath.IsAbs(name) || filepath.Clean(name) != name {
			return errors.New("安全配置需要本地绝对路径")
		}
		if info, err := os.Lstat(name); err != nil || info.Mode().Perm()&0077 != 0 {
			return errors.New("安全配置必须仅 root 可访问（0600）")
		}
		data, err := protectedRead(filepath.Dir(name), filepath.Base(name), 8192)
		if err != nil {
			return errors.New("安全配置必须为 root 所有的普通受保护文件")
		}
		var cfg struct {
			Logs []sc.LogSource `json:"logs"`
		}
		d := json.NewDecoder(strings.NewReader(string(data)))
		d.DisallowUnknownFields()
		if d.Decode(&cfg) != nil || d.Decode(new(any)) != io.EOF || len(cfg.Logs) > 4 {
			return errors.New("安全日志配置无效或超过 4 个文件")
		}
		seen := map[string]bool{}
		for _, s := range cfg.Logs {
			if secureRoot(filepath.Dir(s.Path)) != nil {
				return errors.New("安全日志父目录必须受 root 保护且不能是链接")
			}
			if !regexp.MustCompile(`^/var/log/(auth\.log|secure|syslog|messages|nginx/[A-Za-z0-9_-][A-Za-z0-9_.-]{0,100}\.log)$`).MatchString(s.Path) || strings.Contains(s.Path, "..") || seen[s.Path] {
				return errors.New("仅允许本地固定 /var/log 安全日志，禁止链接和路径穿越")
			}
			if s.Kind != "ssh" && s.Kind != "nginx_access" && s.Kind != "nginx_error" && s.Kind != "system" {
				return errors.New("日志类型无效")
			}
			seen[s.Path] = true
		}
		c.Logs.Sources = cfg.Logs
	}
	if m.websites != nil {
		c.Website = m.securityWebsites
	}
	m.security = c
	return nil
}

// Reuses the v0.3 host-only site allowlist and protected read helpers. It never
// probes arbitrary HTTP paths or duplicates certificate persistence.
func (m *Manager) securityWebsites(ctx context.Context) []shared.SecurityCheck {
	w := m.websites
	w.mu.Lock()
	defer w.mu.Unlock()
	counts := map[string]int{}
	unknown := false
	tlsUnknown := false
	expired := 0
	redirects, redirectUnknown, redirectChecked := 0, false, 0
	for i, site := range w.config.Sites {
		if ctx.Err() != nil {
			unknown = true
			break
		}
		result, e := m.readSite(site)
		if e != nil {
			unknown = true
			continue
		}
		text := regexp.MustCompile(`(?m)#.*$`).ReplaceAllString(result.Config, "")
		for key, p := range map[string]string{"autoindex": `(?m)\bautoindex\s+on\s*;`, "old_tls": `\bssl_protocols\s+[^;]*(?:TLSv1\s|TLSv1\.1)`, "tokens": `\bserver_tokens\s+on\s*;`, "body": `\bclient_max_body_size\s+0\s*;`} {
			if regexp.MustCompile(p).MatchString(text) {
				counts[key]++
			}
		}
		sensitiveSite := len(siteWarnings(site.RootPath)) > 0
		found, complete := securitySensitiveEntries(site.RootPath)
		if len(found) > 0 {
			sensitiveSite = true
		}
		if !complete {
			unknown = true
		}
		if sensitiveSite {
			counts["sensitive"]++
		}
		// Static absence remains unknown: inherited directives and Include are not
		// fully parsed, and CSP/HSTS/frame policy must follow site requirements.
		if !strings.Contains(text, "X-Content-Type-Options") || !strings.Contains(text, "Referrer-Policy") {
			counts["headers"]++
		}
		w.probeMu.Lock()
		check, ok := w.checks[site.Domain]
		w.probeMu.Unlock()
		if !ok || check.CheckedAt == 0 || time.Since(time.UnixMilli(check.CheckedAt)) > 10*time.Minute {
			tlsUnknown = true
		} else if check.Error == "SSL 证书验证失败" {
			expired++
		} else if check.TLS.Status == "有效" && check.TLS.DaysLeft <= 30 {
			expired++
		} else if check.TLS.Status != "有效" {
			tlsUnknown = true
		}
		if i < 2 && ok {
			state := securityRedirect(ctx, site.Domain, check)
			if state == "risk" {
				redirects++
			}
			if state == "unknown" {
				redirectUnknown = true
			}
			redirectChecked++
		} else {
			redirectUnknown = true
		}
	}
	items := []shared.SecurityCheck{}
	for _, v := range []struct{ key, title, sev, why, advice string }{{"autoindex", "Nginx 目录索引", "medium", "显式 autoindex on 可能展示目录列表", "确认业务是否需要后人工关闭"}, {"old_tls", "弱 TLS 配置", "high", "站点文件显式包含旧 TLS 协议", "结合实际有效配置评估禁用旧协议"}, {"tokens", "Nginx 版本暴露", "low", "显式 server_tokens on 暴露版本信息", "人工评估是否隐藏版本"}, {"body", "请求体限制", "low", "显式无限请求体可能扩大资源消耗", "按上传业务设置合理上限"}, {"sensitive", "网站敏感文件与权限", "medium", "v0.3 本地静态检查提示风险，不证明公网可下载", "核对 .env/.git、备份、日志及访问拒绝规则，不批量 HTTP 探测"}, {"headers", "安全响应头与代理", "info", "静态文件未覆盖所有响应头、继承与反向代理行为", "按业务核对 nosniff、Referrer-Policy、CSP、HSTS、Frame 与代理头；不要盲用固定策略"}} {
		state, sev := "unknown", "info"
		if counts[v.key] > 0 && v.key != "headers" {
			state, sev = "risk", v.sev
		}
		ev := v.why + "；匹配站点数=" + strconv.Itoa(counts[v.key])
		if unknown {
			ev += "；部分站点无法读取"
		}
		items = append(items, sc.Check("website."+v.key, "website", v.title, state, sev, ev, "静态检查不是 Nginx 完整配置求值或漏洞确认", v.advice))
	}
	state, sev := "pass", "info"
	if tlsUnknown || unknown {
		state = "unknown"
	}
	if expired > 0 {
		state, sev = "risk", "high"
	}
	items = append(items, sc.Check("website.tls", "website", "证书与 HTTPS 状态", state, sev, "复用 v0.3 已校验的证书/域名检查；异常或 30 天内到期站点="+strconv.Itoa(expired), "没有建立额外证书表；过期、链异常和主机名问题以 v0.3 结果为准", "在 SSL 页面核对每个站点；HTTP→HTTPS 跳转链与过期缓存需人工确认"))
	state, sev = "pass", "info"
	if redirectUnknown {
		state = "unknown"
	}
	if redirects > 0 {
		state, sev = "risk", "low"
	}
	items = append(items, sc.Check("website.redirect", "website", "HTTP 到 HTTPS 跳转", state, sev, "低频 HEAD / 检查="+strconv.Itoa(redirectChecked)+"；未跳转至同域 HTTPS="+strconv.Itoa(redirects), "每次最多 2 个已授权站点，固定 v0.3 已验证公网地址，不跟随跳转", "按业务确认是否强制 HTTPS；未探测敏感文件，未检查完整跳转链"))
	php := m.phpInspect(ctx)
	state = "unknown"
	sev = "info"
	if !php.Available {
		if _, e := m.Run(ctx, "php", "-n", "-v"); errors.Is(e, exec.ErrNotFound) {
			state = "not_installed"
		}
	} else if len(php.Warnings) > 0 {
		state, sev = "risk", "medium"
	}
	items = append(items, sc.Check("website.php", "website", "PHP 配置与维护状态", state, sev, php.Version+" "+strings.Join(php.Warnings, ", "), "复用 CLI/FPM 检测；CLI 不代表全部 FPM 池，版本维护状态待发行版确认", "核对 display_errors/expose_php、FPM 有效用户和站点权限；不因版本号断言漏洞"))
	processCtx, processCancel := context.WithTimeout(ctx, time.Second)
	defer processCancel()
	processes, e := m.Run(processCtx, "ps", "-eo", "euid,comm")
	rootCount, otherCount := 0, 0
	if e == nil {
		for _, line := range strings.Split(string(processes), "\n") {
			f := strings.Fields(line)
			if len(f) == 2 && strings.Contains(f[1], "php-fpm") {
				if f[0] == "0" {
					rootCount++
				} else {
					otherCount++
				}
			}
		}
	}
	items = append(items, sc.Check("website.fpm_identity", "website", "PHP-FPM 可见进程权限", "unknown", "info", "可见 root 进程="+strconv.Itoa(rootCount)+"；非 root 进程="+strconv.Itoa(otherCount), "master 常以 root 运行，不能据此判定 worker 也为 root；进程权限可能不完整", "在主机核对 FPM pool 的实际 user/group、运行状态及网站目录权限"))
	return items
}

// Metadata only, one directory level and 128 entries at most. Never reads a
// sensitive file's contents or recursively walks the website tree.
func securitySensitiveEntries(root string) ([]string, bool) {
	if secureRoot(root) != nil {
		return nil, false
	}
	handle, e := os.OpenRoot(root)
	if e != nil {
		return nil, false
	}
	defer handle.Close()
	f, e := handle.Open(".")
	if e != nil {
		return nil, false
	}
	defer f.Close()
	entries, e := f.ReadDir(129)
	if e != nil && e != io.EOF {
		return nil, false
	}
	found := []string{}
	for _, entry := range entries {
		name := strings.ToLower(entry.Name())
		sensitive := name == ".env" || name == ".git" || name == "config.php" || name == "wp-config.php"
		for _, suffix := range []string{".sql", ".sql.gz", ".dump", ".bak", ".old", ".zip", ".tar.gz", ".log", ".ini", ".conf"} {
			sensitive = sensitive || strings.HasSuffix(name, suffix)
		}
		if sensitive && len(found) < 8 {
			found = append(found, shared.SecurityText(entry.Name(), 80))
		}
	}
	return found, len(entries) <= 128
}

func securityRedirect(ctx context.Context, domain string, check shared.WebsiteCheck) string {
	if !shared.ValidDomain(domain) || check.Domain != domain || time.Since(time.UnixMilli(check.CheckedAt)) > 10*time.Minute {
		return "unknown"
	}
	ips := append(append([]string{}, check.A...), check.AAAA...)
	if len(ips) == 0 || len(ips) > 32 {
		return "unknown"
	}
	for _, ip := range ips {
		if !publicAddress(net.ParseIP(ip)) {
			return "unknown"
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	transport := &http.Transport{DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		if address != net.JoinHostPort(domain, "80") {
			return nil, errors.New("unexpected target")
		}
		var last error
		for _, ip := range ips {
			conn, e := (&net.Dialer{Timeout: time.Second}).DialContext(ctx, "tcp", net.JoinHostPort(ip, "80"))
			if e == nil {
				return conn, nil
			}
			last = e
		}
		return nil, last
	}, ResponseHeaderTimeout: time.Second, MaxResponseHeaderBytes: 8192, DisableKeepAlives: true}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	req, _ := http.NewRequestWithContext(ctx, "HEAD", "http://"+domain+"/", nil)
	resp, e := client.Do(req)
	if e != nil {
		return "unknown"
	}
	defer resp.Body.Close()
	location, e := url.Parse(resp.Header.Get("Location"))
	if e == nil && resp.StatusCode >= 300 && resp.StatusCode < 400 && location.Scheme == "https" && location.Hostname() == domain && (location.Port() == "" || location.Port() == "443") {
		return "pass"
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		return "risk"
	}
	return "unknown"
}
