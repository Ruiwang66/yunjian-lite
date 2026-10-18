// Package api exposes the authenticated control plane and a separately authenticated Agent endpoint.
package api

import (
	"cloudease-lite/server/config"
	"cloudease-lite/server/models"
	"cloudease-lite/server/service"
	live "cloudease-lite/server/websocket"
	"cloudease-lite/shared"
	"errors"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type attempt struct {
	Count int
	Until time.Time
}
type API struct {
	DB         *gorm.DB
	Config     config.Config
	Hub        *live.Hub
	loginMu    sync.Mutex
	attempts   map[string]attempt
	limiter    *requestLimiter
	loginSlots chan struct{}
}

func New(db *gorm.DB, cfg config.Config, hub *live.Hub) *API {
	return &API{DB: db, Config: cfg, Hub: hub, attempts: map[string]attempt{}, limiter: newLimiter(), loginSlots: make(chan struct{}, 4)}
}
func fail(c *gin.Context, status int, message string) {
	c.AbortWithStatusJSON(status, gin.H{"error": message})
}
func dbError(c *gin.Context) { fail(c, 500, "数据库操作失败，请检查服务日志") }
func found(c *gin.Context, err error) bool {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		fail(c, 404, "服务器不存在")
		return false
	}
	if err != nil {
		dbError(c)
		return false
	}
	return true
}

func (a *API) Router() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.CustomRecoveryWithWriter(io.Discard, func(c *gin.Context, _ any) {
		log.Print("HTTP handler panic recovered")
		fail(c, 500, "服务暂时不可用")
	}))
	if err := r.SetTrustedProxies(a.Config.TrustedProxies); err != nil {
		panic(err)
	}
	r.Use(a.securityHeaders)
	r.GET("/healthz", func(c *gin.Context) {
		sqlDB, err := a.DB.DB()
		if err != nil || sqlDB.Ping() != nil {
			fail(c, 503, "database unavailable")
			return
		}
		c.JSON(200, gin.H{"status": "ok", "version": shared.Version})
	})
	r.POST("/api/auth/login", a.audit, a.login)
	r.GET("/uninstall-client.sh", serveInstaller("uninstall.sh"))
	r.GET("/uninstall-client.ps1", serveInstaller("uninstall.ps1"))
	r.GET("/install-client.sh", serveInstaller("agent.sh"))
	r.GET("/install-client.ps1", serveInstaller("agent.ps1"))
	r.GET("/install-agent.sh", serveInstaller("agent.sh"))
	r.GET("/install-agent.ps1", serveInstaller("agent.ps1"))
	agentInstall := r.Group("/api/agent")
	agentInstall.Use(a.agentInstallAuth)
	agentInstall.POST("/report", a.report)
	agentInstall.GET("/operations/next", a.nextOperation)
	agentInstall.POST("/operations/result", a.operationResult)
	agentInstall.GET("/check", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true, "version": shared.Version, "management_protocol": 1, "website_protocol": 1})
	})
	agentInstall.GET("/downloads/:file", a.downloadAgent)
	agentInstall.GET("/downloads/:file/sha256", a.downloadChecksum)
	r.GET("/api/public/servers", a.publicServers)
	r.GET("/api/public/servers/:id", a.publicServer)
	r.GET("/api/public/servers/:id/metrics", a.history)
	r.GET("/api/public/ws", func(c *gin.Context) {
		a.Hub.Serve(c.Writer, c.Request, time.Now().Add(24*time.Hour), func() bool { return true }, c.ClientIP())
	})
	private := r.Group("/api")
	private.Use(a.auth, a.audit, a.permissions)
	a.managementRoutes(private)
	a.websiteRoutes(private)
	private.GET("/auth/me", a.me)
	private.POST("/auth/logout", a.logout)
	private.POST("/auth/password", a.password)
	private.GET("/servers", a.servers)
	private.POST("/servers", a.createServer)
	private.GET("/servers/:id", a.server)
	private.PATCH("/servers/:id", a.renameServer)
	private.PATCH("/servers/:id/location", a.updateLocation)
	private.DELETE("/servers/:id", a.deleteServer)
	private.GET("/servers/:id/uninstall", a.uninstallInfo)
	private.POST("/servers/:id/token", a.rotateToken)
	private.GET("/servers/:id/metrics", a.history)
	private.GET("/logs", a.logs)
	private.GET("/ws", func(c *gin.Context) {
		session := c.MustGet("session").(models.Session)
		a.Hub.Serve(c.Writer, c.Request, session.ExpiresAt, func() bool {
			var count int64
			return a.DB.Model(&models.Session{}).Joins("JOIN users ON users.id = sessions.user_id").Where("sessions.id = ? AND sessions.expires_at > ?", session.ID, time.Now().UTC()).Count(&count).Error == nil && count == 1
		}, c.ClientIP())
	})
	private.GET("/downloads/:file", a.downloadAgent)
	private.GET("/downloads/:file/sha256", a.downloadChecksum)
	r.Static("/assets", filepath.Join(a.Config.WebDir, "assets"))
	r.NoRoute(func(c *gin.Context) {
		if c.Request.Method != "GET" || strings.HasPrefix(c.Request.URL.Path, "/api") || strings.HasPrefix(c.Request.URL.Path, "/assets/") {
			fail(c, 404, "接口不存在")
			return
		}
		c.File(filepath.Join(a.Config.WebDir, "index.html"))
	})
	return r
}
func (a *API) auth(c *gin.Context) {
	raw, err := c.Cookie("cloudease_session")
	if err != nil || len(raw) > 128 {
		fail(c, 401, "请先登录")
		return
	}
	var s models.Session
	err = a.DB.Select("sessions.*").Joins("JOIN users ON users.id = sessions.user_id").Where("sessions.token_hash = ? AND sessions.expires_at > ?", service.HashToken(raw), time.Now().UTC()).First(&s).Error
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			dbError(c)
		} else {
			fail(c, 401, "登录已过期，请重新登录")
		}
		return
	}
	var u models.User
	if a.DB.First(&u, s.UserID).Error != nil {
		dbError(c)
		return
	}
	c.Set("user", u)
	c.Set("session", s)
	c.Next()
}
func (a *API) cookie(c *gin.Context, value string, maxAge int) {
	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie("cloudease_session", value, maxAge, "/", "", a.Config.CookieSecure, true)
}

// Rate limiting has bounded memory and a 15-minute window per source IP.
func (a *API) allowLogin(ip string) bool {
	a.loginMu.Lock()
	defer a.loginMu.Unlock()
	now := time.Now()
	for key, value := range a.attempts {
		if now.After(value.Until) {
			delete(a.attempts, key)
		}
	}
	v := a.attempts[ip]
	if v.Count >= 10 || len(a.attempts) >= 10000 {
		return false
	}
	if v.Count == 0 {
		v.Until = now.Add(15 * time.Minute)
	}
	v.Count++
	a.attempts[ip] = v
	return true
}
