// Package security performs bounded local read-only checks, with no listener,
// shell evaluator, remediation action or vulnerability database.
package security

import (
	"cloudease-lite/shared"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

type Runner func(context.Context, string, ...string) ([]byte, error)
type Collector struct {
	Run     Runner
	Read    func(string) ([]byte, error)
	Stat    func(string) (os.FileInfo, error)
	Website func(context.Context) []shared.SecurityCheck
	Logs    *LogReader
}

func New(run Runner) *Collector {
	return &Collector{Run: run, Read: readBounded, Stat: os.Lstat, Logs: &LogReader{}}
}
func readBounded(name string) ([]byte, error) {
	f, err := openLog(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 65536 {
		return nil, errors.New("file unavailable or exceeds 64 KiB")
	}
	return io.ReadAll(io.LimitReader(f, 65536))
}
func Check(rule, dimension, title, status, severity, evidence, reason, advice string) shared.SecurityCheck {
	return shared.SecurityCheck{Rule: rule, Dimension: dimension, Title: title, Status: status, Severity: severity, Evidence: shared.SecurityText(evidence, 600), Reason: shared.SecurityText(reason, 450), Advice: shared.SecurityText(advice, 800)}
}
func (c *Collector) command(ctx context.Context, name string, args ...string) (string, error) {
	child, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
	defer cancel()
	b, err := c.Run(child, name, args...)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
func (c *Collector) Scan(ctx context.Context, o shared.SecurityOptions) shared.SecurityReport {
	r := shared.SecurityReport{StartedAt: time.Now().UnixMilli(), Checks: []shared.SecurityCheck{}, Events: []shared.SecurityObservation{}, Scope: "仅客户端可见的系统/网络/文件；容器与 systemd 隔离可能改变视图，公网可达性未验证"}
	r.Checks = append(r.Checks, c.system(ctx)...)
	r.Checks = append(r.Checks, c.ssh(ctx)...)
	r.Checks = append(r.Checks, c.network(ctx)...)
	r.Checks = append(r.Checks, c.docker(ctx)...)
	if c.Website != nil {
		r.Checks = append(r.Checks, c.Website(ctx)...)
	} else {
		r.Checks = append(r.Checks, Check("website.authorization", "website", "网站本地授权", "not_authorized", "info", "未配置 v0.3 网站白名单", "未读取未授权网站", "由主机管理员明确授权站点"))
	}
	if c.Logs != nil {
		checks, events := c.Logs.Analyze(ctx, o)
		r.Checks = append(r.Checks, checks...)
		r.Events = append(r.Events, events...)
	}
	r.FinishedAt = time.Now().UnixMilli()
	return r
}
func (c *Collector) system(ctx context.Context) []shared.SecurityCheck {
	items := []shared.SecurityCheck{}
	osinfo, err := c.Read("/etc/os-release")
	if err != nil {
		osinfo, err = c.Read("/usr/lib/os-release")
	}
	kernel, kerr := c.Read("/proc/sys/kernel/osrelease")
	status := "pass"
	if err != nil || kerr != nil {
		status = "unknown"
	}
	pretty := ""
	for _, line := range strings.Split(string(osinfo), "\n") {
		if strings.HasPrefix(line, "PRETTY_NAME=") {
			pretty = strings.Trim(line[12:], `"`)
		}
	}
	if pretty == "" || strings.TrimSpace(string(kernel)) == "" {
		status = "unknown"
	}
	items = append(items, Check("system.identity", "system", "操作系统与内核", status, "info", pretty+" / "+strings.TrimSpace(string(kernel)), "版本只作资产信息，未根据版本号断言 CVE", "结合发行版安全公告和补丁回移植确认漏洞"))
	priv, pe := c.Read("/proc/self/status")
	state, ev := "unknown", "无法读取客户端运行身份"
	if pe == nil {
		fields := []string{}
		for _, line := range strings.Split(string(priv), "\n") {
			if strings.HasPrefix(line, "Uid:") || strings.HasPrefix(line, "Gid:") || strings.HasPrefix(line, "CapEff:") {
				fields = append(fields, strings.Join(strings.Fields(line), " "))
			}
		}
		if len(fields) > 0 {
			state = "pass"
			ev = strings.Join(fields, "; ")
		}
	}
	items = append(items, Check("system.runtime", "system", "客户端运行权限", state, "info", ev, "仅展示实际运行身份和有效能力，不自动提升权限", "运维和安全检测需明确授权，普通及兼容模式应保持原权限"))
	for _, rule := range []struct{ key, path string }{{"aslr", "/proc/sys/kernel/randomize_va_space"}, {"kptr", "/proc/sys/kernel/kptr_restrict"}, {"dmesg", "/proc/sys/kernel/dmesg_restrict"}} {
		data, e := c.Read(rule.path)
		state, sev, value := "unknown", "info", strings.TrimSpace(string(data))
		if e == nil {
			if value == "0" {
				state, sev = "risk", "medium"
			} else if value == "1" || value == "2" {
				state = "pass"
			}
		}
		items = append(items, Check("system."+rule.key, "system", "内核安全配置："+rule.key, state, sev, rule.path+"="+value, "读取当前可见内核参数，容器中可能共享宿主机设置", "结合调试和业务需求人工评估，不自动修改 sysctl"))
	}
	up, e := c.Read("/var/lib/update-notifier/updates-available")
	if e != nil {
		items = append(items, Check("system.updates", "system", "可用安全更新", "unknown", "info", "无可用本地摘要，未联网刷新软件源", "无法确认缓存新鲜度与安全补丁状态", "使用发行版包管理器核对安全更新及缓存时间"))
	} else {
		state, sev := "unknown", "info"
		if strings.Contains(string(up), "security") && !strings.Contains(string(up), "0 of these updates") {
			state, sev = "risk", "medium"
		}
		items = append(items, Check("system.updates", "system", "本地更新摘要", state, sev, string(up), "本地缓存不能证明已修复或存在特定 CVE", "核对缓存时间后人工安排安全更新"))
	}
	passwd, e := c.Read("/etc/passwd")
	if e != nil {
		items = append(items, Check("system.uid0", "system", "UID 0 账户", "unknown", "info", "无法读取账户列表", "权限或文件不可用", "在主机本地核对"))
	} else {
		names := []string{}
		for _, line := range strings.Split(string(passwd), "\n") {
			f := strings.Split(line, ":")
			if len(f) > 2 && f[2] == "0" {
				names = append(names, f[0])
			}
		}
		state, sev := "pass", "info"
		if len(names) == 0 {
			state = "unknown"
		} else if len(names) != 1 || len(names) == 1 && names[0] != "root" {
			state, sev = "risk", "high"
		}
		items = append(items, Check("system.uid0", "system", "UID 0 账户", state, sev, strings.Join(names, ", "), "额外 UID 0 账户具有等同 root 权限", "确认用途及备用管理入口后人工清理额外特权账户"))
	}
	for _, v := range []struct {
		key, path string
		mask      os.FileMode
	}{{"system.sudoers", "/etc/sudoers", 0022}, {"system.ssh_permissions", "/etc/ssh/sshd_config", 0022}, {"system.tmp_permissions", "/tmp", 0002}} {
		info, e := c.Stat(v.path)
		state, sev, ev := "unknown", "info", "无法读取元数据"
		if e == nil {
			ev = fmt.Sprintf("%s mode=%s", v.path, info.Mode())
			state = "pass"
			if info.Mode()&os.ModeSymlink != 0 {
				state = "unknown"
			}
			if info.Mode().Perm()&v.mask != 0 && !(v.path == "/tmp" && info.Mode()&os.ModeSticky != 0) {
				state, sev = "risk", "high"
			}
		}
		items = append(items, Check(v.key, "system", v.path+" 权限", state, sev, ev, "只检查固定关键路径，不遍历整个文件系统", "人工核对所有者、写权限及业务用途"))
	}
	sudo, e := c.Read("/etc/sudoers")
	sstate, sev, ev := "unknown", "info", "sudoers 不可读或 Include/组继承未完整解析"
	if e == nil && strings.Contains(string(sudo), "NOPASSWD:") {
		sstate, sev, ev = "risk", "medium", "主配置存在 NOPASSWD；未输出账户、命令或凭据内容"
	}
	items = append(items, Check("system.sudo_policy", "system", "sudo 管理授权", sstate, sev, ev, "静态提示不代表完整有效策略", "用 visudo -c 和管理员账户核对有效 sudo 规则"))
	suid := []string{}
	checked := 0
	partial := false
	for _, p := range []string{"/bin/bash", "/bin/sh", "/usr/bin/python3", "/usr/bin/vim", "/usr/bin/find", "/usr/bin/curl"} {
		if st, e := c.Stat(p); e == nil && st.Mode()&os.ModeSymlink == 0 {
			checked++
			if st.Mode()&(os.ModeSetuid|os.ModeSetgid) != 0 {
				suid = append(suid, p)
			}
		} else if e == nil || !os.IsNotExist(e) {
			partial = true
		}
	}
	state, sev = "pass", "info"
	if checked == 0 || partial {
		state = "unknown"
	}
	if len(suid) > 0 {
		state, sev = "risk", "high"
	}
	items = append(items, Check("system.suid_scope", "system", "固定高风险程序 SUID/SGID", state, sev, fmt.Sprintf("检查 %d/6 个固定路径；%s", checked, strings.Join(suid, ", ")), "解释器或文件工具的特殊位可能扩大权限；不是全盘审计", "核对软件包原始权限，不自动 chmod"))
	return items
}
func (c *Collector) ssh(ctx context.Context) []shared.SecurityCheck {
	out, err := c.command(ctx, "sshd", "-T")
	if err != nil {
		status := "unknown"
		if errors.Is(err, exec.ErrNotFound) {
			status = "not_installed"
		}
		return []shared.SecurityCheck{Check("ssh.effective", "network", "SSH 有效配置", status, "info", "标准路径 sshd -T 不可用、权限不足或配置验证失败", "不使用单个文件猜测 Include/Match 生效结果；CLI 缺失不证明其他实例不存在", "主机本地验证 sshd -T；仅覆盖默认连接上下文")}
	}
	values := map[string]string{}
	for _, l := range strings.Split(out, "\n") {
		f := strings.Fields(l)
		if len(f) >= 2 {
			values[f[0]] = f[1]
		}
	}
	items := []shared.SecurityCheck{}
	for _, v := range []struct{ key, title, want, sev string }{{"passwordauthentication", "SSH 密码登录", "no", "medium"}, {"pubkeyauthentication", "SSH 公钥登录", "yes", "medium"}, {"permitemptypasswords", "SSH 空密码", "no", "high"}, {"maxauthtries", "SSH 最大认证次数", "6", "low"}} {
		got, ok := values[v.key]
		state, sev := "pass", "info"
		if !ok {
			state = "unknown"
		} else if v.key == "maxauthtries" {
			var n int
			if _, e := fmt.Sscan(got, &n); e != nil {
				state = "unknown"
			} else if n > 6 {
				state, sev = "risk", v.sev
			}
		} else if got != v.want {
			state, sev = "risk", v.sev
		}
		items = append(items, Check("ssh."+v.key, "network", v.title, state, sev, v.key+"="+got+"；sshd -T 默认上下文（含 Include）", "Match 随来源、用户等变化，未覆盖所有连接", "先确认密钥和备用入口，备份并验证配置，新连接成功后再结束旧连接"))
	}
	state, sev := "pass", "info"
	if values["permitrootlogin"] == "" || values["passwordauthentication"] == "" {
		state = "unknown"
	} else if values["permitrootlogin"] == "yes" && values["passwordauthentication"] == "yes" {
		state, sev = "risk", "high"
	}
	return append(items, Check("ssh.root_password", "network", "root 密码远程登录", state, sev, "permitrootlogin="+values["permitrootlogin"]+" passwordauthentication="+values["passwordauthentication"], "默认上下文允许 root 密码时，凭据失陷可直接获得管理权限", "确认普通管理账户和 Key 正常后人工限制 root 密码登录；核对 Match"))
}
