package api

import (
	"cloudease-lite/server/models"
	"cloudease-lite/shared"
	"github.com/gin-gonic/gin"
	"strings"
)

func (a *API) updateLocation(c *gin.Context) {
	var in struct {
		CountryCode string `json:"country_code"`
	}
	if !decodeJSON(c, &in) {
		return
	}
	code := strings.ToUpper(strings.TrimSpace(in.CountryCode))
	if !shared.ValidCountryCode(code) {
		fail(c, 400, "请选择支持的国家或地区")
		return
	}
	var s models.Server
	if !found(c, a.DB.First(&s, "id = ?", c.Param("id")).Error) {
		return
	}
	if a.DB.Model(&s).Update("country_code", code).Error != nil {
		dbError(c)
		return
	}
	s.CountryCode = code
	c.Set("audit_server_id", s.ID)
	a.Hub.Broadcast("servers_changed")
	c.JSON(200, a.view(s))
}
