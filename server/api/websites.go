package api

import (
	"cloudease-lite/server/models"
	"cloudease-lite/server/service"
	"cloudease-lite/shared"
	"encoding/json"
	"errors"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"strconv"
	"strings"
	"time"
)

func (a *API) websiteRoutes(r *gin.RouterGroup) {
	r.GET("/websites", a.websites)
	r.POST("/websites", a.saveWebsite(false))
	r.GET("/websites/:id", a.website)
	r.PATCH("/websites/:id", a.saveWebsite(true))
	r.DELETE("/websites/:id", a.deleteWebsite)
	r.POST("/websites/:id/check", a.websiteTask("website.check"))
	r.GET("/nginx/status", a.webEnvironment("nginx"))
	r.GET("/nginx/sites", a.websiteTask("nginx.sites"))
	r.GET("/nginx/site/config", a.websiteTask("nginx.read"))
	r.POST("/nginx/site/enable", a.websiteTask("nginx.enable"))
	r.POST("/nginx/site/disable", a.websiteTask("nginx.disable"))
	r.GET("/php/info", a.webEnvironment("php"))
	r.POST("/php/inspect", a.websiteTask("php.inspect"))
	r.GET("/ssl", a.websiteRecords("ssl"))
	r.POST("/ssl/check", a.websiteTask("ssl.check"))
	r.GET("/domain", a.websiteRecords("domain"))
	r.POST("/domain/check", a.websiteTask("domain.check"))
	r.GET("/backup", a.websiteRecords("backup"))
	r.POST("/backup", a.websiteTask("backup"))
	r.GET("/backup/schedule", func(c *gin.Context) {
		c.JSON(200, gin.H{"enabled": false, "supported": false, "message": "自动备份接口预留，本版仅手动执行"})
	})
}
func websiteSpec(w models.Website) shared.WebsiteSpec {
	return shared.WebsiteSpec{Name: w.Name, Domain: w.Domain, RootPath: w.RootPath, NginxConfig: w.NginxConfig}
}
func cancelWebsiteTasks(tx *gorm.DB, id uint) error {
	var tasks []models.ManagementTask
	if e := tx.Where("website_id = ? AND status = ?", id, "queued").Find(&tasks).Error; e != nil {
		return e
	}
	for _, task := range tasks {
		now := time.Now().UTC()
		if e := tx.Model(&task).Updates(map[string]any{"status": "cancelled", "request_json": "", "updated_at": now}).Error; e != nil {
			return e
		}
		if e := tx.Model(&models.OperationLog{}).Where("id = ?", task.OperationLogID).Updates(map[string]any{"status": "cancelled", "updated_at": now}).Error; e != nil {
			return e
		}
		if e := tx.Model(&models.BackupTask{}).Where("task_id = ?", task.ID).Updates(map[string]any{"status": "cancelled", "updated_at": now}).Error; e != nil {
			return e
		}
	}
	return nil
}
func inventory(s models.Server) *shared.WebsiteInventory {
	var info shared.ManagementInfo
	json.Unmarshal([]byte(s.ManagementJSON), &info)
	return info.Websites
}
func allowedWebsite(s models.Server, spec shared.WebsiteSpec) bool {
	info := inventory(s)
	if info == nil || !info.Enabled {
		return false
	}
	for _, item := range info.Sites {
		if item.Domain == spec.Domain && item.RootPath == spec.RootPath && item.NginxConfig == spec.NginxConfig {
			return true
		}
	}
	return false
}
func (a *API) websites(c *gin.Context) {
	s, ok := a.managedServer(c, c.Query("server_id"))
	if !ok {
		return
	}
	items := []models.Website{}
	if a.DB.Where("server_id = ?", s.ID).Order("id desc").Limit(20).Find(&items).Error != nil {
		dbError(c)
		return
	}
	web := inventory(s)
	if web != nil {
		for _, w := range items {
			var h models.HealthCheck
			var ssl models.SSLCertificate
			var d models.Domain
			if a.DB.Where("website_id = ?", w.ID).First(&h).Error != nil {
				continue
			}
			a.DB.Where("website_id = ?", w.ID).First(&ssl)
			a.DB.Where("website_id = ?", w.ID).First(&d)
			check := shared.WebsiteCheck{Domain: w.Domain, Status: h.Status, HTTPStatus: h.HTTPStatus, ResponseMS: h.ResponseMS, Accessible: h.Accessible, Error: h.Error, CheckedAt: h.UpdatedAt.UnixMilli(), TLS: shared.TLSInfo{Status: ssl.Status, Issuer: ssl.Issuer, NotBefore: ssl.NotBefore, NotAfter: ssl.NotAfter, DaysLeft: ssl.DaysLeft, Version: ssl.TLSVersion, Warning: ssl.Warning, Error: ssl.Error}}
			json.Unmarshal([]byte(d.AJSON), &check.A)
			json.Unmarshal([]byte(d.AAAAJSON), &check.AAAA)
			json.Unmarshal([]byte(h.WarningsJSON), &check.Warnings)
			json.Unmarshal([]byte(ssl.ChainJSON), &check.TLS.Chain)
			matched := false
			for i, old := range web.Checks {
				if old.Domain == w.Domain {
					if check.CheckedAt >= old.CheckedAt {
						web.Checks[i] = check
					}
					matched = true
					break
				}
			}
			if !matched {
				web.Checks = append(web.Checks, check)
			}
		}
	}
	c.JSON(200, gin.H{"items": items, "inventory": web})
}
func (a *API) loadWebsite(c *gin.Context, id string) (models.Website, bool) {
	var w models.Website
	if !numericID(id) {
		fail(c, 400, "网站编号无效")
		return w, false
	}
	if !found(c, a.DB.First(&w, "id = ?", id).Error) {
		return w, false
	}
	return w, true
}
func (a *API) website(c *gin.Context) {
	w, ok := a.loadWebsite(c, c.Param("id"))
	if !ok {
		return
	}
	var health models.HealthCheck
	var ssl models.SSLCertificate
	var domain models.Domain
	a.DB.Where("website_id = ?", w.ID).First(&health)
	a.DB.Where("website_id = ?", w.ID).First(&ssl)
	a.DB.Where("website_id = ?", w.ID).First(&domain)
	c.JSON(200, gin.H{"website": w, "health": health, "ssl": ssl, "domain": domain})
}
func (a *API) saveWebsite(edit bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		var in struct {
			shared.WebsiteSpec
			ServerID uint `json:"server_id"`
		}
		if !decodeJSON(c, &in) {
			return
		}
		in.Name = strings.TrimSpace(in.Name)
		if !shared.ValidWebsite(in.WebsiteSpec) || strings.ContainsAny(in.Name, "\x00\r\n") {
			fail(c, 400, "网站名称、域名或路径无效")
			return
		}
		s, ok := a.managedServer(c, strconv.FormatUint(uint64(in.ServerID), 10))
		if !ok {
			return
		}
		if !allowedWebsite(s, in.WebsiteSpec) {
			fail(c, 403, "站点未在 v0.3 Linux 客户端本地白名单中授权")
			return
		}
		row := models.Website{ServerID: s.ID, Name: in.Name, Domain: in.Domain, RootPath: in.RootPath, NginxConfig: in.NginxConfig, Status: "未检测"}
		if edit {
			old, ok := a.loadWebsite(c, c.Param("id"))
			if !ok {
				return
			}
			if old.ServerID != s.ID {
				fail(c, 400, "不能改变网站所属服务器")
				return
			}
			row.ID = old.ID
			row.CreatedAt = old.CreatedAt
		}
		if info := inventory(s); info != nil {
			row.PHPVersion = info.PHP.Version
		}
		err := a.DB.Transaction(func(tx *gorm.DB) error {
			if edit {
				if e := cancelWebsiteTasks(tx, row.ID); e != nil {
					return e
				}
				for _, model := range []any{&models.HealthCheck{}, &models.SSLCertificate{}, &models.Domain{}} {
					if e := tx.Where("website_id = ?", row.ID).Delete(model).Error; e != nil {
						return e
					}
				}
				return tx.Save(&row).Error
			}
			var count int64
			tx.Model(&models.Website{}).Where("server_id = ?", s.ID).Count(&count)
			if count >= 20 {
				return errors.New("website limit")
			}
			return tx.Create(&row).Error
		})
		if err != nil {
			if !c.IsAborted() {
				fail(c, 409, "网站重复、数量超限或无法保存")
			}
			return
		}
		c.Set("audit_server_id", s.ID)
		code := 201
		if edit {
			code = 200
		}
		c.JSON(code, row)
	}
}
func (a *API) deleteWebsite(c *gin.Context) {
	w, ok := a.loadWebsite(c, c.Param("id"))
	if !ok {
		return
	}
	err := a.DB.Transaction(func(tx *gorm.DB) error {
		if e := cancelWebsiteTasks(tx, w.ID); e != nil {
			return e
		}
		for _, model := range []any{&models.HealthCheck{}, &models.SSLCertificate{}, &models.Domain{}} {
			if e := tx.Where("website_id = ?", w.ID).Delete(model).Error; e != nil {
				return e
			}
		}
		return tx.Delete(&w).Error
	})
	if err != nil {
		dbError(c)
		return
	}
	c.Set("audit_server_id", w.ServerID)
	c.JSON(200, gin.H{"ok": true, "message": "只删除控制台网站记录；主机文件、Nginx 配置和备份保留"})
}
func (a *API) webEnvironment(kind string) gin.HandlerFunc {
	return func(c *gin.Context) {
		s, ok := a.managedServer(c, c.Query("server_id"))
		if !ok {
			return
		}
		info := inventory(s)
		if info == nil {
			fail(c, 409, "请配置 v0.3 Linux 客户端的网站白名单")
			return
		}
		if kind == "php" {
			c.JSON(200, info.PHP)
		} else {
			c.JSON(200, info.Nginx)
		}
	}
}
func (a *API) websiteRecords(kind string) gin.HandlerFunc {
	return func(c *gin.Context) {
		s, ok := a.managedServer(c, c.Query("server_id"))
		if !ok {
			return
		}
		var model any
		switch kind {
		case "ssl":
			model = &[]models.SSLCertificate{}
		case "domain":
			model = &[]models.Domain{}
		case "backup":
			model = &[]models.BackupTask{}
		}
		if a.DB.Where("server_id = ?", s.ID).Order("id desc").Limit(200).Find(model).Error != nil {
			dbError(c)
			return
		}
		c.JSON(200, gin.H{"items": model})
	}
}
func (a *API) websiteTask(baseAction string) gin.HandlerFunc {
	return func(c *gin.Context) {
		action := baseAction
		var in struct {
			ServerID  uint   `json:"server_id"`
			WebsiteID uint   `json:"website_id"`
			Kind      string `json:"kind"`
		}
		if c.Request.Method == "POST" {
			if !decodeJSON(c, &in) {
				return
			}
		} else {
			n, e := strconv.ParseUint(c.Query("server_id"), 10, 32)
			if e != nil {
				fail(c, 400, "服务器编号无效")
				return
			}
			in.ServerID = uint(n)
			if raw := c.Query("website_id"); raw != "" {
				n, e = strconv.ParseUint(raw, 10, 32)
				if e != nil {
					fail(c, 400, "网站编号无效")
					return
				}
				in.WebsiteID = uint(n)
			}
		}
		if action == "backup" {
			if in.Kind != "files" && in.Kind != "database" {
				fail(c, 400, "备份类型须为 files 或 database")
				return
			}
			action = "backup." + in.Kind
		}
		s, ok := a.managedServer(c, strconv.FormatUint(uint64(in.ServerID), 10))
		if !ok {
			return
		}
		if s.OS != "linux" || a.view(s).Status != "online" || inventory(s) == nil || !inventory(s).Enabled {
			fail(c, 409, "请连接在线的 v0.3 Linux 客户端并配置网站白名单")
			return
		}
		req := shared.ManagementRequest{Action: action}
		var w models.Website
		if action != "nginx.sites" && action != "nginx.status" && action != "php.inspect" {
			w, ok = a.loadWebsite(c, strconv.FormatUint(uint64(in.WebsiteID), 10))
			if !ok {
				return
			}
			if w.ServerID != s.ID || !allowedWebsite(s, websiteSpec(w)) {
				fail(c, 403, "网站不属于此服务器或白名单已变化")
				return
			}
			spec := websiteSpec(w)
			req.Website = &spec
			req.Target = w.Domain
		}
		u := c.MustGet("user").(models.User)
		now := time.Now().UTC()
		id, e := service.RandomToken()
		if e != nil {
			dbError(c)
			return
		}
		raw, _ := json.Marshal(req)
		ttl := 45 * time.Second
		if strings.HasPrefix(action, "backup.") {
			ttl = 120 * time.Second
		}
		task := models.ManagementTask{ID: id, ServerID: s.ID, WebsiteID: w.ID, UserID: u.ID, SessionID: c.MustGet("session").(models.Session).ID, KeyHash: s.Token, Action: action, RequestJSON: string(raw), Status: "queued", CreatedAt: now, UpdatedAt: now, ExpiresAt: now.Add(ttl), OperationLogID: c.MustGet("operation_log").(uint)}
		err := a.DB.Transaction(func(tx *gorm.DB) error {
			var count int64
			if e := tx.Model(&models.ManagementTask{}).Where("server_id = ? AND status IN ? AND expires_at > ?", s.ID, []string{"queued", "running"}, now).Count(&count).Error; e != nil {
				return e
			}
			if count >= 4 {
				return errors.New("busy")
			}
			if e := tx.Create(&task).Error; e != nil {
				return e
			}
			if strings.HasPrefix(action, "backup.") {
				if e := tx.Create(&models.BackupTask{WebsiteID: w.ID, ServerID: s.ID, TaskID: id, Kind: in.Kind, Name: w.Name, Status: "queued", CreatedAt: now, UpdatedAt: now}).Error; e != nil {
					return e
				}
			}
			return tx.Model(&models.OperationLog{}).Where("id = ?", task.OperationLogID).Updates(map[string]any{"server_id": s.ID, "action": action, "target": req.Target, "status": "queued", "updated_at": now}).Error
		})
		if err != nil {
			fail(c, 503, "运维队列繁忙，请稍后重试")
			return
		}
		c.Set("audit_handled", true)
		c.JSON(202, gin.H{"id": id, "status": "queued", "expires_at": task.ExpiresAt})
	}
}
func cacheWebsiteCheck(tx *gorm.DB, s models.Server, w models.Website, check shared.WebsiteCheck, now time.Time) error {
	if !validWebsiteCheck(check) || check.Domain != w.Domain {
		return errors.New("invalid website check")
	}
	a, _ := json.Marshal(check.A)
	aaaa, _ := json.Marshal(check.AAAA)
	chain, _ := json.Marshal(check.TLS.Chain)
	warnings, _ := json.Marshal(check.Warnings)
	rows := []any{&models.Domain{WebsiteID: w.ID, ServerID: s.ID, Domain: w.Domain, Status: check.Status, AJSON: string(a), AAAAJSON: string(aaaa), UpdatedAt: now}, &models.SSLCertificate{WebsiteID: w.ID, ServerID: s.ID, Domain: w.Domain, Status: check.TLS.Status, Issuer: check.TLS.Issuer, NotBefore: check.TLS.NotBefore, NotAfter: check.TLS.NotAfter, DaysLeft: check.TLS.DaysLeft, TLSVersion: check.TLS.Version, ChainJSON: string(chain), Warning: check.TLS.Warning, Error: check.TLS.Error, UpdatedAt: now}, &models.HealthCheck{WebsiteID: w.ID, ServerID: s.ID, Domain: w.Domain, Status: check.Status, HTTPStatus: check.HTTPStatus, ResponseMS: check.ResponseMS, Accessible: check.Accessible, Error: check.Error, WarningsJSON: string(warnings), UpdatedAt: now}}
	for _, row := range rows {
		if e := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "website_id"}}, UpdateAll: true}).Create(row).Error; e != nil {
			return e
		}
	}
	return tx.Model(&w).Updates(map[string]any{"status": check.Status, "updated_at": now}).Error
}
func validWebsiteCheck(check shared.WebsiteCheck) bool {
	if !shared.ValidDomain(check.Domain) || len(check.A)+len(check.AAAA) > 32 || len(check.TLS.Chain) > 8 || len(check.Warnings) > 20 || len(check.Error) > 512 || len(check.TLS.Error) > 512 || len(check.TLS.Issuer) > 512 || len(check.TLS.Warning) > 240 || len(check.Status) > 32 || len(check.TLS.Status) > 32 || len(check.TLS.Version) > 32 || check.HTTPStatus < 0 || check.HTTPStatus > 599 || check.ResponseMS < 0 || check.ResponseMS > 120000 {
		return false
	}
	for _, v := range append(append(append([]string{}, check.A...), check.AAAA...), append(check.TLS.Chain, check.Warnings...)...) {
		if len(v) > 1024 {
			return false
		}
	}
	return true
}
func cacheWebsiteResult(tx *gorm.DB, task models.ManagementTask, data json.RawMessage, now time.Time) error {
	var w models.Website
	if task.WebsiteID != 0 && tx.First(&w, "id = ? AND server_id = ?", task.WebsiteID, task.ServerID).Error != nil {
		return nil
	}
	if strings.HasPrefix(task.Action, "backup.") {
		var result shared.BackupResult
		if json.Unmarshal(data, &result) != nil || result.Kind != strings.TrimPrefix(task.Action, "backup.") || result.Size < 0 || result.Size > 256*1024*1024 || len(result.Name) > 240 || strings.ContainsAny(result.Name, "/\\\x00") || len(result.SHA256) != 64 {
			return errors.New("invalid backup result")
		}
		return tx.Model(&models.BackupTask{}).Where("task_id = ?", task.ID).Updates(map[string]any{"name": result.Name, "size": result.Size, "sha256": result.SHA256, "status": "success", "updated_at": now}).Error
	}
	if task.Action == "website.check" || task.Action == "ssl.check" || task.Action == "domain.check" {
		var check shared.WebsiteCheck
		if json.Unmarshal(data, &check) != nil {
			return errors.New("invalid check")
		}
		var request shared.ManagementRequest
		json.Unmarshal([]byte(task.RequestJSON), &request)
		if request.Website == nil || w.Domain != request.Website.Domain || w.RootPath != request.Website.RootPath || w.NginxConfig != request.Website.NginxConfig {
			return nil
		}
		return cacheWebsiteCheck(tx, models.Server{ID: task.ServerID}, w, check, now)
	}
	return nil
}
