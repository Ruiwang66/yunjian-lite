package api

import (
	"cloudease-lite/server/models"
	"cloudease-lite/server/service"
	"cloudease-lite/shared"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"strconv"
	"time"
)

func (a *API) securityRoutes(r *gin.RouterGroup) {
	r.GET("/security/overview", a.securityOverview)
	r.GET("/security/servers", a.securityServers)
	r.POST("/security/scans", a.securityScan)
	r.GET("/security/scans", a.securityScans)
	r.GET("/security/scans/:id", a.securityReport)
	r.GET("/security/reports/:id", a.securityReport)
	r.GET("/security/findings", a.securityFindings)
	r.GET("/security/findings/:id", a.securityFinding)
	r.PATCH("/security/findings/:id", a.securityReview)
	r.GET("/security/events", a.securityEvents)
	r.GET("/security/alerts", a.securityAlerts)
	r.PATCH("/security/alerts/:id", a.securityReadAlert)
	r.GET("/security/scores", a.securityScores)
	r.GET("/security/rules", a.securityRules)
	r.PUT("/security/rules", a.securitySaveRules)
}
func (a *API) securityServer(c *gin.Context) (models.Server, bool) {
	return a.managedServer(c, c.Query("server_id"))
}
func securityQuery(c *gin.Context, db *gorm.DB, id uint) (*gorm.DB, bool) {
	days, e := strconv.Atoi(c.DefaultQuery("days", "7"))
	page, e2 := strconv.Atoi(c.DefaultQuery("page", "1"))
	if e != nil || days < 1 || days > 365 || e2 != nil || page < 1 || page > 10000 {
		fail(c, 400, "时间范围或页码无效")
		return db, false
	}
	q := db.Where("server_id = ? AND created_at >= ?", id, time.Now().UTC().Add(-time.Duration(days)*24*time.Hour))
	if severity := c.Query("severity"); severity != "" {
		if !shared.SecuritySeverity(severity) {
			fail(c, 400, "风险等级无效")
			return db, false
		}
		switch db.Statement.Model.(type) {
		case *models.SecurityScan, *models.SecurityScore:
		default:
			q = q.Where("severity = ?", severity)
		}
	}
	return q.Offset((page - 1) * 50).Limit(50), true
}

func (a *API) securityServers(c *gin.Context) {
	var servers []models.Server
	if a.DB.Order("id").Limit(200).Find(&servers).Error != nil {
		dbError(c)
		return
	}
	var scores []models.SecurityScore
	if a.DB.Where("id IN (?)", a.DB.Model(&models.SecurityScore{}).Select("MAX(id)").Group("server_id")).Find(&scores).Error != nil {
		dbError(c)
		return
	}
	byID := map[uint]models.SecurityScore{}
	for _, s := range scores {
		byID[s.ServerID] = s
	}
	items := []gin.H{}
	for _, s := range servers {
		score := byID[s.ID]
		items = append(items, gin.H{"server_id": s.ID, "name": s.Name, "online": s.LastSeen != nil && time.Since(*s.LastSeen) <= a.Config.OfflineAfter, "score": score.Score, "risk": score.Risk, "checked_at": score.CreatedAt})
	}
	c.JSON(200, gin.H{"items": items, "limit": 200})
}
func (a *API) securityScan(c *gin.Context) {
	var in struct {
		ServerID uint `json:"server_id"`
	}
	if !decodeJSON(c, &in) {
		return
	}
	u := c.MustGet("user").(models.User)
	scan, err := service.QueueSecurity(a.DB, in.ServerID, u.ID, c.MustGet("session").(models.Session).ID, c.MustGet("operation_log").(uint), time.Now().UTC())
	if err != nil {
		fail(c, 409, "无法安排扫描："+shared.SecurityText(err.Error(), 240))
		return
	}
	c.Set("audit_handled", true)
	a.Hub.Broadcast("security_changed")
	c.JSON(202, scan)
}
func (a *API) securityOverview(c *gin.Context) {
	s, ok := a.securityServer(c)
	if !ok {
		return
	}
	var score models.SecurityScore
	var scan models.SecurityScan
	if e := a.DB.Where("server_id = ?", s.ID).Order("id desc").Limit(1).Find(&score).Error; e != nil {
		dbError(c)
		return
	}
	if e := a.DB.Where("server_id = ?", s.ID).Order("id desc").Limit(1).Find(&scan).Error; e != nil {
		dbError(c)
		return
	}
	var info shared.ManagementInfo
	_ = json.Unmarshal([]byte(s.ManagementJSON), &info)
	var unread int64
	u := c.MustGet("user").(models.User)
	if a.DB.Model(&models.SecurityAlert{}).Where("server_id = ? AND id NOT IN (?)", s.ID, a.DB.Model(&models.SecurityAlertRead{}).Select("alert_id").Where("user_id = ?", u.ID)).Count(&unread).Error != nil {
		dbError(c)
		return
	}
	c.JSON(200, gin.H{"server_id": s.ID, "enabled": info.Enabled && info.SecurityEnabled && s.OS == "linux", "online": a.view(s).Status == "online", "latest_scan": scan, "latest_score": score, "unread_alerts": unread, "scope": "仅当前客户端可见范围；分数为内部规则参考值，不是安全保证"})
}
func (a *API) securityScans(c *gin.Context) {
	s, ok := a.securityServer(c)
	if !ok {
		return
	}
	q, ok := securityQuery(c, a.DB.Model(&models.SecurityScan{}), s.ID)
	if !ok {
		return
	}
	rows := []models.SecurityScan{}
	if q.Order("id desc").Find(&rows).Error != nil {
		dbError(c)
		return
	}
	c.JSON(200, gin.H{"items": rows})
}
func (a *API) securityReport(c *gin.Context) {
	s, ok := a.securityServer(c)
	if !ok {
		return
	}
	if !numericID(c.Param("id")) {
		fail(c, 400, "报告编号无效")
		return
	}
	var scan models.SecurityScan
	if !found(c, a.DB.First(&scan, "id = ? AND server_id = ?", c.Param("id"), s.ID).Error) {
		return
	}
	var report any
	var options shared.SecurityOptions
	_ = json.Unmarshal([]byte(scan.ReportJSON), &report)
	_ = json.Unmarshal([]byte(scan.OptionsJSON), &options)
	c.JSON(200, gin.H{"scan": scan, "report": report, "options": options, "score_is_reference_only": true})
}
func (a *API) securityFindings(c *gin.Context) {
	s, ok := a.securityServer(c)
	if !ok {
		return
	}
	q, ok := securityQuery(c, a.DB.Model(&models.SecurityFinding{}), s.ID)
	if !ok {
		return
	}
	if id := c.Query("scan_id"); id != "" {
		if !numericID(id) {
			fail(c, 400, "扫描编号无效")
			return
		}
		q = q.Where("scan_id = ?", id)
	} else {
		q = q.Where("scan_id IN (?)", a.DB.Model(&models.SecurityScan{}).Select("MAX(id)").Where("server_id = ? AND status = ?", s.ID, "success"))
	}
	rows := []models.SecurityFinding{}
	if q.Order("id desc").Find(&rows).Error != nil {
		dbError(c)
		return
	}
	c.JSON(200, gin.H{"items": rows})
}
func (a *API) securityFinding(c *gin.Context) {
	s, ok := a.securityServer(c)
	if !ok {
		return
	}
	if !numericID(c.Param("id")) {
		fail(c, 400, "发现编号无效")
		return
	}
	var f models.SecurityFinding
	if !found(c, a.DB.First(&f, "id = ? AND server_id = ?", c.Param("id"), s.ID).Error) {
		return
	}
	c.JSON(200, f)
}
func (a *API) securityReview(c *gin.Context) {
	var in struct {
		ServerID uint   `json:"server_id"`
		Status   string `json:"status"`
	}
	if !decodeJSON(c, &in) {
		return
	}
	if !numericID(c.Param("id")) {
		fail(c, 400, "发现编号无效")
		return
	}
	if in.Status != "pending" && in.Status != "confirmed" && in.Status != "resolved" && in.Status != "false_positive" {
		fail(c, 400, "处理状态无效")
		return
	}
	var f models.SecurityFinding
	if !found(c, a.DB.First(&f, "id = ? AND server_id = ?", c.Param("id"), in.ServerID).Error) {
		return
	}
	// Assessment and original evidence remain immutable. Only workflow changes.
	u := c.MustGet("user").(models.User)
	if a.DB.Model(&f).Updates(map[string]any{"workflow": in.Status, "reviewed_by": u.Username, "updated_at": time.Now().UTC()}).Error != nil {
		dbError(c)
		return
	}
	c.Set("audit_server_id", in.ServerID)
	a.Hub.Broadcast("security_changed")
	c.JSON(200, gin.H{"ok": true})
}
func (a *API) securityEvents(c *gin.Context) {
	s, ok := a.securityServer(c)
	if !ok {
		return
	}
	q, ok := securityQuery(c, a.DB.Model(&models.SecurityEvent{}), s.ID)
	if !ok {
		return
	}
	rows := []models.SecurityEvent{}
	if q.Order("id desc").Find(&rows).Error != nil {
		dbError(c)
		return
	}
	c.JSON(200, gin.H{"items": rows})
}
func (a *API) securityAlerts(c *gin.Context) {
	s, ok := a.securityServer(c)
	if !ok {
		return
	}
	q, ok := securityQuery(c, a.DB.Model(&models.SecurityAlert{}), s.ID)
	if !ok {
		return
	}
	rows := []models.SecurityAlert{}
	if q.Order("id desc").Find(&rows).Error != nil {
		dbError(c)
		return
	}
	reads := []models.SecurityAlertRead{}
	ids := []uint{}
	for _, r := range rows {
		ids = append(ids, r.ID)
	}
	if a.DB.Where("user_id = ? AND alert_id IN ?", c.MustGet("user").(models.User).ID, ids).Find(&reads).Error != nil {
		dbError(c)
		return
	}
	c.JSON(200, gin.H{"items": rows, "read": reads})
}
func (a *API) securityReadAlert(c *gin.Context) {
	var in struct {
		ServerID uint `json:"server_id"`
	}
	if !decodeJSON(c, &in) {
		return
	}
	if !numericID(c.Param("id")) {
		fail(c, 400, "告警编号无效")
		return
	}
	var alert models.SecurityAlert
	if !found(c, a.DB.First(&alert, "id = ? AND server_id = ?", c.Param("id"), in.ServerID).Error) {
		return
	}
	row := models.SecurityAlertRead{AlertID: alert.ID, UserID: c.MustGet("user").(models.User).ID, ReadAt: time.Now().UTC()}
	if a.DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error != nil {
		dbError(c)
		return
	}
	c.JSON(200, gin.H{"ok": true})
}
func (a *API) securityScores(c *gin.Context) {
	s, ok := a.securityServer(c)
	if !ok {
		return
	}
	q, ok := securityQuery(c, a.DB.Model(&models.SecurityScore{}), s.ID)
	if !ok {
		return
	}
	rows := []models.SecurityScore{}
	if q.Order("id desc").Find(&rows).Error != nil {
		dbError(c)
		return
	}
	c.JSON(200, gin.H{"items": rows})
}
func (a *API) securityRules(c *gin.Context) {
	s, ok := a.securityServer(c)
	if !ok {
		return
	}
	r, o, e := service.SecurityPolicy(a.DB, s.ID)
	if e != nil {
		dbError(c)
		return
	}
	c.JSON(200, gin.H{"policy": r, "options": o, "webhook_enabled": false})
}
func (a *API) securitySaveRules(c *gin.Context) {
	if c.MustGet("user").(models.User).Role != "admin" {
		fail(c, 403, "仅管理员可以修改规则、保留策略和计划")
		return
	}
	var in struct {
		ServerID      uint                   `json:"server_id"`
		Options       shared.SecurityOptions `json:"options"`
		RetentionDays int                    `json:"retention_days"`
		MuteHours     int                    `json:"mute_hours"`
		IntervalHours int                    `json:"interval_hours"`
	}
	if !decodeJSON(c, &in) {
		return
	}
	if !in.Options.Valid() || in.RetentionDays < 1 || in.RetentionDays > 365 || in.MuteHours < 0 || in.MuteHours > 168 || in.IntervalHours != 0 && (in.IntervalHours < 6 || in.IntervalHours > 168) {
		fail(c, 400, "规则参数无效，定时周期为 0 或 6–168 小时")
		return
	}
	var s models.Server
	if !found(c, a.DB.First(&s, in.ServerID).Error) {
		return
	}
	raw, _ := json.Marshal(in.Options)
	now := time.Now().UTC()
	r := models.SecurityRule{ServerID: s.ID, OptionsJSON: string(raw), RetentionDays: in.RetentionDays, IntervalHours: in.IntervalHours, NextScanAt: now.Add(time.Duration(in.IntervalHours) * time.Hour), UserID: c.MustGet("user").(models.User).ID, SessionID: c.MustGet("session").(models.Session).ID, UpdatedAt: now}
	if in.MuteHours > 0 {
		r.MuteUntil = now.Add(time.Duration(in.MuteHours) * time.Hour)
	}
	if a.DB.Clauses(clause.OnConflict{UpdateAll: true}).Create(&r).Error != nil {
		dbError(c)
		return
	}
	c.Set("audit_server_id", s.ID)
	c.JSON(200, gin.H{"ok": true})
}
