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
	return publicProjection(a.view(s))
}
func publicProjection(v serverView) publicServerView {
	return publicServerView{CountryCode: v.CountryCode, ID: v.ID, Name: v.Name, Status: v.Status, OS: v.OS,
		Platform: v.Platform, PlatformVersion: v.PlatformVersion, Arch: v.Arch,
		CPUCores: v.CPUCores, Uptime: v.Uptime, LastSeen: v.LastSeen, Latest: v.Latest}
}
func (a *API) publicServers(c *gin.Context) {
	items, err := a.listViews()
	if err != nil {
		dbError(c)
		return
	}
	result := make([]publicServerView, 0, len(items))
	for _, s := range items {
		result = append(result, publicProjection(s))
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
