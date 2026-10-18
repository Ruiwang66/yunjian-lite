package api

import (
	"cloudease-lite/server/models"
	"cloudease-lite/server/service"
	"cloudease-lite/shared"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"strconv"
	"strings"
	"time"
)

type serverView struct {
	models.Server
	Latest *models.Metric `json:"latest"`
}

func (a *API) view(s models.Server) serverView {
	// Compute current status as well as persisting transitions in the background worker.
	if s.LastSeen == nil {
		s.Status = "pending"
	} else if time.Since(*s.LastSeen) > a.Config.OfflineAfter {
		s.Status = "offline"
	} else {
		s.Status = "online"
	}
	var m models.Metric
	v := serverView{Server: s}
	if a.DB.Where("server_id = ?", s.ID).Order("created_at desc").First(&m).Error == nil {
		v.Latest = &m
	}
	return v
}
func (a *API) servers(c *gin.Context) {
	items := []models.Server{}
	if a.DB.Order("id desc").Find(&items).Error != nil {
		dbError(c)
		return
	}
	result := make([]serverView, 0, len(items))
	for _, s := range items {
		result = append(result, a.view(s))
	}
	c.JSON(200, result)
}
func (a *API) server(c *gin.Context) {
	var s models.Server
	if !found(c, a.DB.First(&s, "id = ?", c.Param("id")).Error) {
		return
	}
	c.JSON(200, a.view(s))
}
func readName(c *gin.Context) (string, bool) {
	var in struct {
		Name string `json:"name"`
	}
	if !decodeJSON(c, &in) {
		return "", false
	}
	name := strings.TrimSpace(in.Name)
	if name == "" || len([]rune(name)) > 80 {
		fail(c, 400, "服务器名称须为 1–80 个字符")
		return "", false
	}
	return name, true
}
func (a *API) createServer(c *gin.Context) {
	var in struct {
		Name        string `json:"name"`
		CountryCode string `json:"country_code"`
	}
	if !decodeJSON(c, &in) {
		return
	}
	name := strings.TrimSpace(in.Name)
	code := strings.ToUpper(strings.TrimSpace(in.CountryCode))
	if name == "" || len([]rune(name)) > 80 {
		fail(c, 400, "服务器名称须为 1–80 个字符")
		return
	}
	if !shared.ValidCountryCode(code) {
		fail(c, 400, "请选择支持的国家或地区")
		return
	}
	raw, err := service.RandomToken()
	if err != nil {
		fail(c, 500, "生成连接密钥失败")
		return
	}
	s := models.Server{Name: name, CountryCode: code, Token: service.HashToken(raw), Status: "pending"}
	if a.DB.Create(&s).Error != nil {
		dbError(c)
		return
	}
	a.Hub.Broadcast("servers_changed")
	c.Set("audit_server_id", s.ID)
	c.JSON(201, gin.H{"server": a.view(s), "token": raw})
}
func (a *API) renameServer(c *gin.Context) {
	name, ok := readName(c)
	if !ok {
		return
	}
	var s models.Server
	if !found(c, a.DB.First(&s, "id = ?", c.Param("id")).Error) {
		return
	}
	if a.DB.Model(&s).Update("name", name).Error != nil {
		dbError(c)
		return
	}
	a.Hub.Broadcast("servers_changed")
	c.JSON(200, a.view(s))
}
func (a *API) rotateToken(c *gin.Context) {
	var s models.Server
	if !found(c, a.DB.First(&s, "id = ?", c.Param("id")).Error) {
		return
	}
	raw, err := service.RandomToken()
	if err != nil {
		fail(c, 500, "生成连接密钥失败")
		return
	}
	if a.DB.Model(&s).Updates(map[string]any{"token": service.HashToken(raw), "last_report_at": int64(0), "last_report_nonce": ""}).Error != nil {
		dbError(c)
		return
	}
	a.Hub.Broadcast("servers_changed")
	c.JSON(200, gin.H{"token": raw})
}

// uninstallInfo supplies only a fingerprint for verifying the local installation.
// The actual key is never returned; the script remains usable after record deletion.
func (a *API) uninstallInfo(c *gin.Context) {
	var s models.Server
	if !found(c, a.DB.First(&s, "id = ?", c.Param("id")).Error) {
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(200, gin.H{"server_id": s.ID, "key_fingerprint": s.Token})
}

func (a *API) deleteServer(c *gin.Context) {
	var s models.Server
	if !found(c, a.DB.First(&s, "id = ?", c.Param("id")).Error) {
		return
	}
	err := a.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.BackupTask{}).Where("server_id = ? AND status IN ?", s.ID, []string{"queued", "running"}).Updates(map[string]any{"status": "cancelled", "updated_at": time.Now().UTC()}).Error; err != nil {
			return err
		}
		if err := tx.Where("server_id = ?", s.ID).Delete(&models.Metric{}).Error; err != nil {
			return err
		}
		if err := tx.Where("server_id = ?", s.ID).Delete(&models.SystemLog{}).Error; err != nil {
			return err
		}
		for _, model := range []any{&models.DockerContainer{}, &models.Service{}, &models.File{}, &models.ManagementTask{}, &models.Website{}, &models.Domain{}, &models.SSLCertificate{}, &models.HealthCheck{}} {
			if err := tx.Where("server_id = ?", s.ID).Delete(model).Error; err != nil {
				return err
			}
		}
		// Preserve operation audit records when the monitored server is removed.
		if err := tx.Model(&models.OperationLog{}).Where("server_id = ? AND status IN ?", s.ID, []string{"queued", "running"}).Updates(map[string]any{"status": "cancelled", "updated_at": time.Now().UTC()}).Error; err != nil {
			return err
		}
		return tx.Delete(&s).Error
	})
	if err != nil {
		dbError(c)
		return
	}
	a.Hub.Broadcast("servers_changed")
	c.JSON(200, gin.H{"ok": true})
}
func (a *API) history(c *gin.Context) {
	var s models.Server
	if !found(c, a.DB.First(&s, "id = ?", c.Param("id")).Error) {
		return
	}
	hours, err := strconv.Atoi(c.DefaultQuery("hours", "6"))
	if err != nil || hours < 1 || hours > 24 {
		fail(c, 400, "hours 须在 1–24 之间")
		return
	}
	items := []models.Metric{}
	// Sample across the full requested window instead of truncating its oldest data.
	bucket := (hours*3600 + 1198) / 1199
	if bucket < 4 {
		bucket = 4
	}
	since := time.Now().UTC().Add(-time.Duration(hours) * time.Hour)
	sub := a.DB.Model(&models.Metric{}).Select("MAX(id)").Where("server_id = ? AND created_at >= ?", s.ID, since).Group("CAST(strftime('%s', created_at) AS INTEGER) / " + strconv.Itoa(bucket))
	if a.DB.Where("id IN (?)", sub).Order("created_at desc").Find(&items).Error != nil {
		dbError(c)
		return
	}
	for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
		items[i], items[j] = items[j], items[i]
	}
	c.JSON(200, items)
}
func (a *API) logs(c *gin.Context) {
	limit, err := strconv.Atoi(c.DefaultQuery("limit", "50"))
	if err != nil || limit < 1 || limit > 100 {
		fail(c, 400, "limit 须在 1–100 之间")
		return
	}
	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil || page < 1 || page > 10000 {
		fail(c, 400, "页码无效")
		return
	}
	q := a.DB.Model(&models.SystemLog{})
	if id := c.Query("server_id"); id != "" {
		if !numericID(id) {
			fail(c, 400, "服务器编号无效")
			return
		}
		q = q.Where("server_id = ?", id)
	}
	if level := c.Query("level"); level != "" {
		if level != "INFO" && level != "WARN" && level != "ERROR" {
			fail(c, 400, "日志等级无效")
			return
		}
		q = q.Where("level = ?", level)
	}
	var total int64
	if q.Count(&total).Error != nil {
		dbError(c)
		return
	}
	items := []models.SystemLog{}
	if q.Order("id desc").Limit(limit).Offset((page-1)*limit).Find(&items).Error != nil {
		dbError(c)
		return
	}
	c.JSON(200, gin.H{"items": items, "total": total, "page": page})
}
