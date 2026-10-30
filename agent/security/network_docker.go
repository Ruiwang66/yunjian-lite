package security

import (
	"cloudease-lite/shared"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

func AddressScope(address string) string {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return "unknown"
	}
	if host == "*" || host == "" {
		return "wildcard"
	}
	if i := strings.IndexByte(host, '%'); i >= 0 {
		host = host[:i]
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return "unknown"
	}
	if ip.IsUnspecified() {
		return "wildcard"
	}
	if ip.IsLoopback() {
		return "loopback"
	}
	if ip.IsPrivate() || ip.IsLinkLocalUnicast() {
		return "private"
	}
	return "public_address"
}
func (c *Collector) network(ctx context.Context) []shared.SecurityCheck {
	items := []shared.SecurityCheck{}
	out, err := c.command(ctx, "ss", "-H", "-l", "-n", "-t", "-u", "-p", "-e")
	if err != nil {
		items = append(items, Check("network.listeners", "network", "TCP/UDP 监听端口", "unknown", "info", "ss 读取失败或权限不足", "无法获取所有进程、权限和服务名称", "核对本机 ss 权限；不需要打开入站端口"))
	} else {
		counts := map[string]int{}
		examples := []string{}
		total := 0
		for _, line := range strings.Split(out, "\n") {
			f := strings.Fields(line)
			if len(f) < 5 {
				continue
			}
			scope := AddressScope(f[4])
			if scope == "unknown" {
				continue
			}
			counts[scope]++
			total++
			if len(examples) < 4 {
				examples = append(examples, shared.SecurityText(line, 150))
			}
		}
		state, sev := "pass", "info"
		if len(out) >= shared.MaxOutputBytes {
			state = "unknown"
		}
		if counts["wildcard"]+counts["public_address"] > 0 {
			state, sev = "risk", "low"
		}
		items = append(items, Check("network.listeners", "network", "TCP/UDP 监听端口", state, sev, fmt.Sprintf("总数=%d 回环=%d 私网=%d 全地址=%d 公网接口=%d；%s", total, counts["loopback"], counts["private"], counts["wildcard"], counts["public_address"], strings.Join(examples, "; ")), "包含地址、进程与可见 uid；全地址监听仅为潜在风险，不证明公网暴露", "结合 NAT、云安全组、防火墙核对；ss 进程权限信息可能因当前权限不完整"))
	}
	states := []string{}
	active := false
	for _, v := range []struct {
		name string
		args []string
	}{{"ufw", []string{"status", "verbose"}}, {"nft", []string{"list", "ruleset"}}, {"iptables", []string{"-S"}}} {
		out, e := c.command(ctx, v.name, v.args...)
		if e != nil {
			states = append(states, v.name+"=不可读取")
			continue
		}
		summary := []string{}
		for _, l := range strings.Split(out, "\n") {
			if strings.Contains(l, "policy") || strings.HasPrefix(l, "-P ") || strings.Contains(l, "Status:") || strings.Contains(l, "Default:") || strings.Contains(l, "dport") || strings.Contains(l, "ACCEPT") {
				if len(summary) < 5 {
					summary = append(summary, strings.TrimSpace(l))
				}
			}
		}
		if v.name == "ufw" && strings.Contains(out, "Status: active") || v.name == "nft" && strings.Contains(out, "hook input") || v.name == "iptables" && strings.Contains(out, "-P INPUT DROP") {
			active = true
		}
		states = append(states, v.name+" "+strings.Join(summary, ", "))
	}
	state := "unknown"
	if active {
		state = "pass"
	}
	items = append(items, Check("network.firewall", "network", "防火墙规则与默认策略", state, "info", strings.Join(states, "; "), "多工具交叉读取，某一工具未启用不代表没有防火墙", "复杂规则、关键端口可达性、NAT 和云安全组需人工联合核对；检测到规则不等于全部端口安全"))
	return items
}

type DockerAudit struct {
	ID             string
	Name           string
	Image          string
	User           string
	Privileged     bool
	NetworkMode    string
	ReadonlyRootfs bool
	CapAdd         []string
	Binds          []string
	Mounts         []struct {
		Source, Destination string
		RW                  bool
	}
	Ports map[string][]struct{ HostIp, HostPort string }
}

func DockerRisks(containers []DockerAudit) []shared.SecurityCheck {
	type rule struct {
		key, title, sev, why, advice string
		bad                          func(DockerAudit) bool
	}
	rules := []rule{
		{"privileged", "特权容器", "critical", "privileged 赋予广泛能力，削弱隔离", "评估业务需求后采用最小权限", func(d DockerAudit) bool { return d.Privileged }},
		{"socket", "Docker Socket 挂载", "high", "可访问 Docker API 的进程可能控制宿主机", "移除不必要挂载或采用权限受限替代", func(d DockerAudit) bool {
			for _, m := range d.Mounts {
				if strings.Contains(m.Source, "docker.sock") || strings.Contains(m.Destination, "docker.sock") {
					return true
				}
			}
			for _, b := range d.Binds {
				if strings.Contains(b, "docker.sock") {
					return true
				}
			}
			return false
		}},
		{"host_network", "host 网络", "medium", "共享主机网络命名空间，需业务确认", "核对隔离需求及实际监听范围", func(d DockerAudit) bool { return d.NetworkMode == "host" }},
		{"sensitive_mount", "敏感宿主机目录挂载", "high", "挂载主机系统树可能扩大凭据读取或修改影响", "只挂载业务必要子目录并优先只读", func(d DockerAudit) bool {
			for _, m := range d.Mounts {
				for _, p := range []string{"/", "/etc", "/root", "/proc", "/sys", "/dev", "/var/lib/docker"} {
					if m.Source == p || p != "/" && strings.HasPrefix(m.Source, p+"/") {
						return true
					}
				}
			}
			return false
		}},
		{"capabilities", "额外 Linux capabilities", "medium", "额外能力可能扩大影响，是否必要依赖业务", "移除不必要 CapAdd，不自动调整运行容器", func(d DockerAudit) bool { return len(d.CapAdd) > 0 }},
		{"root_user", "容器 root 用户", "medium", "未指定用户通常使用镜像默认 root；不等同于宿主机 root", "确认镜像有效用户，尽可能采用非 root", func(d DockerAudit) bool {
			return d.User == "" || strings.Split(d.User, ":")[0] == "0" || strings.Split(d.User, ":")[0] == "root"
		}},
		{"ports", "全接口端口映射", "low", "映射不证明公网可达", "核对防火墙、NAT 与业务需求", func(d DockerAudit) bool {
			for _, v := range d.Ports {
				for _, p := range v {
					if p.HostIp == "" || p.HostIp == "0.0.0.0" || p.HostIp == "::" {
						return true
					}
				}
			}
			return false
		}},
		{"image_tag", "镜像版本约束", "low", "缺少固定标签或摘要可能导致更新不确定", "固定经验证的版本或镜像摘要", func(d DockerAudit) bool {
			if strings.Contains(d.Image, "@sha256:") {
				return false
			}
			last := d.Image[strings.LastIndex(d.Image, "/")+1:]
			return !strings.Contains(last, ":") || strings.HasSuffix(last, ":latest")
		}},
		{"rootfs", "容器根文件系统可写", "low", "是否合理取决于业务写入路径", "评估只读根文件系统及必要独立数据卷", func(d DockerAudit) bool { return !d.ReadonlyRootfs }},
	}
	items := []shared.SecurityCheck{}
	for _, r := range rules {
		names := []string{}
		count := 0
		for _, d := range containers {
			if r.bad(d) {
				count++
				if len(names) < 4 {
					names = append(names, strings.TrimPrefix(d.Name, "/"))
				}
			}
		}
		state, sev := "pass", "info"
		if count > 0 {
			state, sev = "risk", r.sev
		}
		items = append(items, Check("docker."+r.key, "docker", r.title, state, sev, fmt.Sprintf("%d/%d 容器：%s", count, len(containers), strings.Join(names, ", ")), r.why, r.advice))
	}
	return items
}
func (c *Collector) docker(ctx context.Context) []shared.SecurityCheck {
	out, err := c.command(ctx, "docker", "ps", "--all", "--no-trunc", "--format", "{{.ID}}")
	if err != nil {
		status := "unknown"
		if errors.Is(err, exec.ErrNotFound) {
			status = "not_installed"
		}
		return []shared.SecurityCheck{Check("docker.availability", "docker", "Docker 检测权限", status, "info", "CLI 未安装、守护进程不可用或权限不足", "不会为审计添加 socket 挂载或 docker 组权限", "确需审计时由主机管理员决定授权范围")}
	}
	ids := strings.Fields(out)
	if len(ids) == 0 {
		return []shared.SecurityCheck{Check("docker.empty", "docker", "Docker 容器", "not_installed", "info", "未发现容器", "无容器不扣分", "无需额外操作")}
	}
	if len(ids) > 20 {
		return []shared.SecurityCheck{Check("docker.bounded", "docker", "Docker 容器范围", "unknown", "info", "超过 20 个容器，本次不完成全量审计", "限制扫描资源，避免部分结果伪装完整通过", "在资源充足的独立审计工具中核对")}
	}
	for _, id := range ids {
		if !regexp.MustCompile(`^[a-f0-9]{12,64}$`).MatchString(id) {
			return []shared.SecurityCheck{Check("docker.output", "docker", "Docker 容器信息", "error", "info", "容器列表格式无效", "无法可靠分析", "核对 Docker CLI 输出")}
		}
	}
	format := `{"ID":{{json .Id}},"Name":{{json .Name}},"Image":{{json .Config.Image}},"User":{{json .Config.User}},"Privileged":{{json .HostConfig.Privileged}},"NetworkMode":{{json .HostConfig.NetworkMode}},"ReadonlyRootfs":{{json .HostConfig.ReadonlyRootfs}},"CapAdd":{{json .HostConfig.CapAdd}},"Binds":{{json .HostConfig.Binds}},"Mounts":{{json .Mounts}},"Ports":{{json .HostConfig.PortBindings}}}`
	args := append([]string{"inspect", "--format", format}, ids...)
	out, err = c.command(ctx, "docker", args...)
	if err != nil {
		return []shared.SecurityCheck{Check("docker.inspect", "docker", "Docker 只读 inspect", "unknown", "info", "读取失败或权限不足", "未获取可靠审计字段", "不为审计提升权限")}
	}
	items := []DockerAudit{}
	for _, l := range strings.Split(strings.TrimSpace(out), "\n") {
		var d DockerAudit
		if json.Unmarshal([]byte(l), &d) != nil {
			return []shared.SecurityCheck{Check("docker.inspect", "docker", "Docker 只读 inspect", "error", "info", "结果格式无效或超过输出上限 "+strconv.Itoa(len(out)), "截断不能视为安全", "减少单机容器范围后人工核对")}
		}
		items = append(items, d)
	}
	if len(items) != len(ids) {
		return []shared.SecurityCheck{Check("docker.inspect", "docker", "Docker 只读 inspect", "error", "info", "返回数量不一致", "不使用部分数据评分", "人工核对容器状态")}
	}
	return DockerRisks(items)
}
