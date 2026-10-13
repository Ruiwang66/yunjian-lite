package api

import (
	"cloudease-lite/server/models"
	"cloudease-lite/server/service"
	"embed"
	"errors"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"strings"
)

//go:embed install/agent.sh install/agent.ps1 install/uninstall.sh install/uninstall.ps1
var installers embed.FS

func serveInstaller(name string) gin.HandlerFunc {
	data, err := installers.ReadFile("install/" + name)
	if err != nil {
		panic(err)
	}
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.Data(200, "text/plain; charset=utf-8", data)
	}
}

// Agent installation uses its own Token, never an administrator's browser cookie.
// This middleware only reads the matching server row; it creates no monitoring data.
func (a *API) agentInstallAuth(c *gin.Context) {
	header := c.GetHeader("Authorization")
	if !strings.HasPrefix(header, "Bearer ") || len(header) > 128 {
		fail(c, 401, "连接密钥无效")
		return
	}
	var s models.Server
	tokenHash := service.HashToken(strings.TrimPrefix(header, "Bearer "))
	err := a.DB.Select("id").Where("token = ?", tokenHash).First(&s).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		fail(c, 401, "连接密钥无效")
		return
	}
	if err != nil {
		dbError(c)
		return
	}
	c.Set("agent_server_id", s.ID)
	c.Set("agent_key_hash", tokenHash)
	c.Next()
}
