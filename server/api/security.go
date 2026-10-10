package api

import (
	"encoding/json"
	"errors"
	"github.com/gin-gonic/gin"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

type bucket struct {
	tokens float64
	at     time.Time
}
type requestLimiter struct {
	mu      sync.Mutex
	entries map[string]bucket
	sweep   time.Time
}

func newLimiter() *requestLimiter { return &requestLimiter{entries: map[string]bucket{}} }
func (l *requestLimiter) allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if now.Sub(l.sweep) > time.Minute {
		for k, v := range l.entries {
			if now.Sub(v.at) > 5*time.Minute {
				delete(l.entries, k)
			}
		}
		l.sweep = now
	}
	b, ok := l.entries[key]
	if !ok {
		if len(l.entries) >= 10000 {
			return false
		}
		b = bucket{tokens: 60, at: now}
	}
	b.tokens += now.Sub(b.at).Seconds() * 5
	if b.tokens > 60 {
		b.tokens = 60
	}
	b.at = now
	if b.tokens < 1 {
		l.entries[key] = b
		return false
	}
	b.tokens--
	l.entries[key] = b
	return true
}
func decodeJSON(c *gin.Context, out any) bool {
	media, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || media != "application/json" {
		fail(c, 415, "请求须使用 application/json")
		return false
	}
	d := json.NewDecoder(c.Request.Body)
	d.DisallowUnknownFields()
	if err = d.Decode(out); err == nil {
		var extra any
		if e := d.Decode(&extra); e != io.EOF {
			if e == nil {
				err = errors.New("unexpected trailing JSON")
			} else {
				err = e
			}
		}
	}
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			fail(c, 413, "请求内容过大")
		} else {
			fail(c, 400, "请求数据格式无效")
		}
		return false
	}
	return true
}
func numericID(raw string) bool {
	if raw == "" {
		return false
	}
	for _, c := range raw {
		if c < '0' || c > '9' {
			return false
		}
	}
	id, err := strconv.ParseUint(raw, 10, 63)
	return err == nil && id > 0
}
func requestOrigin(c *gin.Context, public string) string {
	if public != "" {
		return public
	}
	scheme := "http"
	if c.Request.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + c.Request.Host
}
func validOrigin(raw string) bool {
	u, e := url.Parse(raw)
	return e == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Hostname() != "" && u.User == nil && u.Path == "" && u.RawQuery == "" && u.Fragment == "" && u.Host == strings.ToLower(u.Host) && !strings.ContainsAny(raw, "'\" \t\r\n")
}
func (a *API) securityHeaders(c *gin.Context) {
	origin := requestOrigin(c, a.Config.PublicURL)
	if !validOrigin(origin) {
		fail(c, 400, "访问地址格式无效")
		return
	}
	wsOrigin := "ws" + strings.TrimPrefix(origin, "http")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("X-Frame-Options", "DENY")
	c.Header("Referrer-Policy", "same-origin")
	c.Header("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
	c.Header("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self' "+wsOrigin+"; font-src 'self' data:; frame-ancestors 'none'; base-uri 'none'; object-src 'none'; form-action 'self'")
	if strings.HasPrefix(origin, "https://") {
		c.Header("Strict-Transport-Security", "max-age=31536000")
	}
	path := c.Request.URL.Path
	if strings.HasPrefix(path, "/api/") {
		c.Header("Cache-Control", "no-store")
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 65536)
		if a.limiter != nil && !a.limiter.allow(c.ClientIP(), time.Now()) {
			c.Header("Retry-After", "1")
			fail(c, 429, "请求过于频繁，请稍后重试")
			return
		}
	}
	if a.Config.PublicURL != "" && !strings.HasPrefix(path, "/api/agent/") && path != "/healthz" {
		u, _ := url.Parse(a.Config.PublicURL)
		if c.Request.Host != u.Host {
			fail(c, 421, "请使用配置的站点地址访问")
			return
		}
	}
	for _, param := range c.Params {
		if param.Key == "id" && !numericID(param.Value) {
			fail(c, 400, "服务器编号无效")
			return
		}
	}
	if supplied := c.GetHeader("Origin"); strings.EqualFold(c.GetHeader("Upgrade"), "websocket") && supplied != "" && supplied != origin {
		fail(c, 403, "请求来源不允许")
		return
	}
	if c.Request.Method != "GET" && c.Request.Method != "HEAD" && c.Request.Method != "OPTIONS" {
		supplied := c.GetHeader("Origin")
		if (supplied != "" && (!validOrigin(supplied) || supplied != origin)) || c.GetHeader("Sec-Fetch-Site") == "cross-site" {
			fail(c, 403, "请求来源不允许")
			return
		}
	}
	c.Next()
}
