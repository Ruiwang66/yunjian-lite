package security

import (
	"cloudease-lite/shared"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type LogSource struct {
	Path string `json:"path"`
	Kind string `json:"kind"`
}
type logCursor struct {
	info   os.FileInfo
	offset int64
	carry  string
}
type logItem struct {
	at                                           time.Time
	source, key                                  string
	fail, success, request, bad, sensitive, exit bool
}
type LogReader struct {
	Sources []LogSource
	cursors map[string]*logCursor
	recent  []logItem
	Now     func() time.Time
}

var accessTime = regexp.MustCompile(`\[([^\]]+)\]`)
var accessStatus = regexp.MustCompile(`"\s+([1-5][0-9]{2})\s`)
var sourceIP = regexp.MustCompile(`(?:from |^)([0-9a-fA-F:.]+)(?: |$)`)
var sensitivePath = regexp.MustCompile(`(?i)(/\.env|/\.git|wp-config\.php|\.(sql|bak|backup)(?:[? /"\r\n]|$))`)

func digest(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }
func logTime(line string, now time.Time) (time.Time, bool) {
	if m := accessTime.FindStringSubmatch(line); len(m) > 1 {
		if t, e := time.Parse("02/Jan/2006:15:04:05 -0700", m[1]); e == nil {
			return t, true
		}
	}
	f := strings.Fields(line)
	if len(f) > 0 {
		if t, e := time.Parse(time.RFC3339Nano, f[0]); e == nil {
			return t, true
		}
	}
	if len(line) >= 15 {
		if t, e := time.ParseInLocation("2006 Jan _2 15:04:05", fmt.Sprint(now.Year())+" "+line[:15], now.Location()); e == nil {
			if t.After(now.Add(24 * time.Hour)) {
				t = t.AddDate(-1, 0, 0)
			}
			return t, true
		}
	}
	return time.Time{}, false
}
func (r *LogReader) segment(source LogSource, f *os.File, cur *logCursor, now time.Time, o shared.SecurityOptions) (bool, error) {
	info, e := f.Stat()
	if e != nil {
		return false, e
	}
	start := cur.offset
	partial := false
	if info.Size()-start > 65536 {
		start = info.Size() - 65536
		cur.carry = ""
		partial = true
	}
	b := make([]byte, min(int64(65536), max(int64(0), info.Size()-start)))
	n, e := f.ReadAt(b, start)
	if e != nil && e != io.EOF {
		return partial, e
	}
	b = b[:n]
	text := cur.carry + string(b)
	if start > cur.offset {
		if i := strings.IndexByte(text, '\n'); i >= 0 {
			text = text[i+1:]
		} else {
			text = ""
		}
	}
	lines := strings.Split(text, "\n")
	cur.carry = lines[len(lines)-1]
	if len(cur.carry) > 8192 {
		cur.carry = ""
		partial = true
	}
	cur.offset = start + int64(n)
	for i, line := range lines[:len(lines)-1] {
		if len(line) > 8192 {
			partial = true
			continue
		}
		at, ok := logTime(line, now)
		if !ok {
			if line != "" {
				partial = true
			}
			continue
		}
		if at.Before(now.Add(-time.Duration(o.WindowSeconds)*time.Second)) || at.After(now.Add(2*time.Minute)) {
			continue
		}
		it := logItem{at: at, key: digest(source.Path + fmt.Sprint(start, int64(i)) + line)}
		if m := sourceIP.FindStringSubmatch(line); len(m) > 1 {
			it.source = digest(m[1])
		} else {
			it.source = "unspecified"
		}
		low := strings.ToLower(line)
		switch source.Kind {
		case "ssh":
			it.fail = strings.Contains(low, "failed password") || strings.Contains(low, "authentication failure") || strings.Contains(low, "invalid user")
			it.success = strings.Contains(low, "accepted password") || strings.Contains(low, "accepted publickey")
		case "nginx_access":
			it.request = true
			it.sensitive = sensitivePath.MatchString(line)
			if m := accessStatus.FindStringSubmatch(line); len(m) > 1 {
				n, _ := strconv.Atoi(m[1])
				it.bad = n == 401 || n == 403 || n == 404 || n >= 500
			}
		case "nginx_error", "system":
			it.exit = strings.Contains(low, "segfault") || strings.Contains(low, "failed with result 'exit-code'") || strings.Contains(low, "exited on signal")
		}
		r.recent = append(r.recent, it)
	}
	return partial, nil
}
func (r *LogReader) Analyze(ctx context.Context, o shared.SecurityOptions) ([]shared.SecurityCheck, []shared.SecurityObservation) {
	if len(r.Sources) == 0 {
		return []shared.SecurityCheck{Check("logs.authorization", "logs", "安全日志授权", "not_authorized", "info", "未配置主机本地日志白名单", "不会猜测或读取未授权日志", "在 security.json 授权最多 4 个固定日志")}, nil
	}
	now := time.Now()
	if r.Now != nil {
		now = r.Now()
	}
	if r.cursors == nil {
		r.cursors = map[string]*logCursor{}
	}
	partial := false
	read := 0
	newData := false
	kindCount := map[string]int{}
	for _, s := range r.Sources {
		if ctx.Err() != nil {
			partial = true
			break
		}
		f, e := openLog(s.Path)
		if e != nil {
			partial = true
			continue
		}
		info, e := f.Stat()
		if e != nil || !info.Mode().IsRegular() {
			f.Close()
			partial = true
			continue
		}
		cur := r.cursors[s.Path]
		if cur == nil {
			cur = &logCursor{}
			r.cursors[s.Path] = cur
		}
		if cur.info != nil && !os.SameFile(cur.info, info) {
			old, e := openLog(s.Path + ".1")
			if e == nil {
				st, _ := old.Stat()
				if st != nil && os.SameFile(st, cur.info) {
					p, err := r.segment(s, old, cur, now, o)
					partial = partial || p || err != nil
				} else {
					partial = true
				}
				old.Close()
			} else {
				partial = true
			}
			cur.offset = 0
			cur.carry = ""
		}
		if info.Size() < cur.offset {
			cur.offset = 0
			cur.carry = ""
			partial = true
		}
		before := len(r.recent)
		p, e := r.segment(s, f, cur, now, o)
		f.Close()
		cur.info = info
		partial = partial || p || e != nil
		if e == nil {
			read++
			kindCount[s.Kind]++
		}
		newData = newData || len(r.recent) > before
	}
	kept := r.recent[:0]
	for _, it := range r.recent {
		if !it.at.Before(now.Add(-time.Duration(o.WindowSeconds) * time.Second)) {
			kept = append(kept, it)
		}
	}
	r.recent = kept
	if len(kept) > 4096 {
		r.recent = kept[len(kept)-4096:]
		partial = true
	}
	fails, requests := map[string]int{}, map[string]int{}
	bad, total, sensitive, success, exits := 0, 0, 0, 0, 0
	for _, it := range r.recent {
		if it.fail {
			fails[it.source]++
		}
		if it.request {
			requests[it.source]++
			total++
		}
		if it.bad {
			bad++
		}
		if it.sensitive {
			sensitive++
		}
		if it.exit {
			exits++
		}
	}
	for _, it := range r.recent {
		if it.success && fails[it.source] >= 3 {
			success++
		}
	}
	maxFail, maxReq := 0, 0
	for _, n := range fails {
		maxFail = max(maxFail, n)
	}
	for _, n := range requests {
		maxReq = max(maxReq, n)
	}
	checks := []shared.SecurityCheck{Check("logs.coverage", "logs", "日志增量读取范围", "unknown", "info", fmt.Sprintf("读取 %d/%d 个日志；窗口 %ds；单文件 64 KiB；截断/轮转丢失/格式不支持=%t", read, len(r.Sources), o.WindowSeconds, partial), "日志覆盖范围有限，异常行为不等于确认攻击", "权限不足、超限、copytruncate 与非标准格式需人工核对")}
	events := []shared.SecurityObservation{}
	for _, v := range []struct {
		rule, title  string
		n, threshold int
		sev          string
		covered      bool
	}{{"logs.ssh_failures", "短时 SSH 登录失败", maxFail, o.SSHFailures, "high", kindCount["ssh"] > 0}, {"logs.requests", "同来源频繁请求", maxReq, o.Requests, "medium", kindCount["nginx_access"] > 0}, {"logs.http_errors", "异常 HTTP 状态比例", bad * 100 / max(1, total), o.ErrorPercent, "medium", kindCount["nginx_access"] > 0 && total >= 10}, {"logs.sensitive", "敏感路径访问", sensitive, o.SensitiveRequests, "high", kindCount["nginx_access"] > 0}, {"logs.login_sequence", "登录失败后成功", success, 1, "high", kindCount["ssh"] > 0}, {"logs.service_exit", "服务异常退出", exits, 1, "medium", kindCount["system"]+kindCount["nginx_error"] > 0}} {
		state, sev := "pass", "info"
		if partial || !v.covered {
			state = "unknown"
		}
		if v.n >= v.threshold {
			state, sev = "risk", v.sev
		}
		msg := fmt.Sprintf("窗口 %ds：观测值=%d，阈值=%d", o.WindowSeconds, v.n, v.threshold)
		checks = append(checks, Check(v.rule, "logs", v.title, state, sev, msg, "规则仅表示异常；代理和 NAT 可能聚合多个用户", "结合业务、可信代理、登录来源人工核对，不自动封禁"))
		if state == "risk" && newData {
			seed := ""
			for _, it := range r.recent {
				if seed == "" {
					seed = it.key
				}
			}
			events = append(events, shared.SecurityObservation{Key: digest(v.rule + seed), Rule: v.rule, Severity: sev, Message: v.title + "；" + msg + "；不是确认攻击", ObservedAt: now.UnixMilli()})
		}
	}
	return checks, events
}
