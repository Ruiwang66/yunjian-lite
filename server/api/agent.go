package api

import (
	"cloudease-lite/server/models"
	"cloudease-lite/server/service"
	"cloudease-lite/shared"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"math"
	"net"
	"strings"
	"time"
)

var errTooFast = errors.New("report too fast")
var errReplay = errors.New("report replay")

func validNonce(nonce string) bool {
	if len(nonce) != 22 {
		return false
	}
	for _, c := range nonce {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-') {
			return false
		}
	}
	return true
}
func validReport(r shared.Report) bool {
	for _, v := range []uint64{r.MemoryTotal, r.MemoryUsed, r.DiskTotal, r.DiskUsed, r.NetworkRX, r.NetworkTX, r.Uptime} {
		if v > math.MaxInt64 {
			return false
		}
	}
	if (r.SentAt == 0) != (r.Nonce == "") || r.SentAt < 0 || (r.Nonce != "" && !validNonce(r.Nonce)) {
		return false
	}
	for _, v := range []float64{r.CPU, r.Memory, r.Disk} {
		if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v > 100 {
			return false
		}
	}
	for _, v := range []float64{r.NetworkRXRate, r.NetworkTXRate} {
		if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
			return false
		}
	}
	return len(r.Name) <= 240 && len(r.OS) <= 64 && len(r.Platform) <= 128 && len(r.PlatformVersion) <= 128 && len(r.Arch) <= 32 && r.CPUCores >= 0 && r.CPUCores <= 4096 && r.MemoryUsed <= r.MemoryTotal && r.DiskUsed <= r.DiskTotal && (r.IP == "" || net.ParseIP(r.IP) != nil)
}
func (a *API) report(c *gin.Context) {
	header := c.GetHeader("Authorization")
	if !strings.HasPrefix(header, "Bearer ") || len(header) > 128 {
		fail(c, 401, "连接密钥无效")
		return
	}
	token := strings.TrimPrefix(header, "Bearer ")
	var report shared.Report
	if !decodeJSON(c, &report) {
		return
	}
	if !validReport(report) {
		fail(c, 400, "监控数据格式无效")
		return
	}
	now := time.Now().UTC()
	if a.Config.RequireReportFreshness && (report.SentAt == 0 || len(report.Nonce) != 22) {
		fail(c, 400, "请升级监控客户端：上报需要时间戳和随机标识")
		return
	}
	if report.SentAt != 0 && (report.SentAt < now.Add(-5*time.Minute).UnixMilli() || report.SentAt > now.Add(5*time.Minute).UnixMilli()) {
		fail(c, 400, "上报时间偏差超过五分钟，请同步系统时钟")
		return
	}
	var id uint
	err := a.DB.Transaction(func(tx *gorm.DB) error {
		var s models.Server
		if err := tx.Where("token = ?", service.HashToken(token)).First(&s).Error; err != nil {
			return err
		}
		id = s.ID
		if report.SentAt != 0 && (report.SentAt <= s.LastReportAt || report.Nonce == s.LastReportNonce) {
			return errReplay
		}
		if s.LastSeen != nil && now.Sub(*s.LastSeen) < 3*time.Second {
			return errTooFast
		}
		wasConnected := s.LastSeen != nil && now.Sub(*s.LastSeen) <= a.Config.OfflineAfter && s.Status == "online"
		ip := report.IP
		if ip == "" {
			ip = c.ClientIP()
		}
		updates := map[string]any{"ip": ip, "status": "online", "os": report.OS, "platform": report.Platform, "platform_version": report.PlatformVersion, "arch": report.Arch, "cpu_cores": report.CPUCores, "uptime": report.Uptime, "last_seen": now}
		if report.SentAt != 0 {
			updates["last_report_at"] = report.SentAt
			updates["last_report_nonce"] = report.Nonce
		}
		if err := tx.Model(&s).Updates(updates).Error; err != nil {
			return err
		}
		if !wasConnected {
			if err := tx.Create(&models.SystemLog{ServerID: s.ID, Level: "INFO", Message: "Agent connected", CreatedAt: now}).Error; err != nil {
				return err
			}
		}
		var previous models.Metric
		_ = tx.Where("server_id = ?", s.ID).Order("created_at desc").First(&previous).Error
		for _, v := range []struct {
			Name              string
			Current, Previous float64
		}{{"CPU", report.CPU, previous.CPU}, {"Memory", report.Memory, previous.Memory}, {"Disk", report.Disk, previous.Disk}} {
			if v.Current >= 90 && v.Previous < 90 {
				if err := tx.Create(&models.SystemLog{ServerID: s.ID, Level: "WARN", Message: fmt.Sprintf("%s usage high (%.1f%%)", v.Name, v.Current), CreatedAt: now}).Error; err != nil {
					return err
				}
			}
			if v.Current < 90 && v.Previous >= 90 {
				if err := tx.Create(&models.SystemLog{ServerID: s.ID, Level: "INFO", Message: fmt.Sprintf("%s usage recovered (%.1f%%)", v.Name, v.Current), CreatedAt: now}).Error; err != nil {
					return err
				}
			}
		}
		sample := models.Sample(s.ID, report, now)
		return tx.Create(&sample).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		fail(c, 401, "连接密钥无效")
		return
	}
	if errors.Is(err, errReplay) {
		fail(c, 409, "重复或过期的上报已拒绝")
		return
	}
	if errors.Is(err, errTooFast) {
		fail(c, 429, "上报过于频繁")
		return
	}
	if err != nil {
		dbError(c)
		return
	}
	a.Hub.Broadcast("metrics_updated")
	c.JSON(200, gin.H{"ok": true, "server_id": id, "next_report_seconds": 4})
}
