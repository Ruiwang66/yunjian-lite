package api

import (
	"cloudease-lite/server/models"
	"github.com/gin-gonic/gin"
	"time"
)

// Explicit projection keeps IP addresses and administration data private.
type publicServerView struct {
	CountryCode     string         `json:"country_code"`
	ID              uint           `json:"id"`
	Name            string         `json:"name"`
	Status          string         `json:"status"`
	OS              string         `json:"os"`
	Platform        string         `json:"platform"`
	PlatformVersion string         `json:"platform_version"`
	Arch            string         `json:"arch"`
	CPUCores        int            `json:"cpu_cores"`
	Uptime          uint64         `json:"uptime"`
	LastSeen        *time.Time     `json:"last_seen"`
	Latest          *models.Metric `json:"latest"`
}

func (a *API) publicView(s models.Server) publicServerView {
	v := a.view(s)
	return publicServerView{CountryCode: s.CountryCode, ID: s.ID, Name: s.Name, Status: v.Status, OS: s.OS,
		Platform: s.Platform, PlatformVersion: s.PlatformVersion, Arch: s.Arch,
		CPUCores: s.CPUCores, Uptime: s.Uptime, LastSeen: s.LastSeen, Latest: v.Latest}
}
func (a *API) publicServers(c *gin.Context) {
	items := []models.Server{}
	if a.DB.Order("id desc").Find(&items).Error != nil {
		dbError(c)
		return
	}
	result := make([]publicServerView, 0, len(items))
	for _, s := range items {
		result = append(result, a.publicView(s))
	}
	c.JSON(200, result)
}
func (a *API) publicServer(c *gin.Context) {
	var s models.Server
	if !found(c, a.DB.First(&s, "id = ?", c.Param("id")).Error) {
		return
	}
	c.JSON(200, a.publicView(s))
}
