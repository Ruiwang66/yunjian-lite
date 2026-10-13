package api

import (
	"cloudease-lite/server/models"
	"cloudease-lite/shared"
	"github.com/gin-gonic/gin"
	"strconv"
	"strings"
	"time"
)

func validRole(role string) bool { return role == "admin" || role == "operator" || role == "viewer" }
func (a *API) permissions(c *gin.Context) {
	u := c.MustGet("user").(models.User)
	path := c.Request.URL.Path
	write := c.Request.Method != "GET" && c.Request.Method != "HEAD"
	own := path == "/api/auth/logout" || path == "/api/auth/password"
	if !validRole(u.Role) || (write && !own && u.Role == "viewer") || (strings.HasPrefix(path, "/api/users") || path == "/api/roles") && u.Role != "admin" {
		fail(c, 403, "当前角色无权执行此操作")
		return
	}
	c.Next()
}
func (a *API) audit(c *gin.Context) {
	path := c.Request.URL.Path
	tracked := c.Request.Method != "GET" || strings.HasPrefix(path, "/api/users") || path == "/api/roles" || strings.HasPrefix(path, "/api/docker/") || strings.HasPrefix(path, "/api/service/") || strings.HasPrefix(path, "/api/files/") || path == "/api/system/info"
	if !tracked {
		c.Next()
		return
	}
	user := "未登录"
	if raw, ok := c.Get("user"); ok {
		user = raw.(models.User).Username
	}
	now := time.Now().UTC()
	entry := models.OperationLog{User: user, IP: shared.SafeLabel(c.ClientIP()), Action: c.Request.Method + " " + shared.SafeLabel(c.FullPath()), Target: shared.SafeLabel(c.Param("id")), Status: "requested", CreatedAt: now, UpdatedAt: now}
	rawID := c.Query("server_id")
	if strings.HasPrefix(path, "/api/servers/") {
		rawID = c.Param("id")
	}
	if numericID(rawID) {
		n, _ := strconv.ParseUint(rawID, 10, 63)
		entry.ServerID = uint(n)
	}
	// Refuse to mutate if audit persistence is unavailable.
	if a.DB.Create(&entry).Error != nil {
		dbError(c)
		return
	}
	c.Set("operation_log", entry.ID)
	c.Next()
	if handled, _ := c.Get("audit_handled"); handled == true {
		return
	}
	status := "success"
	if c.Writer.Status() >= 400 {
		status = "failed"
	}
	updates := map[string]any{"status": status, "updated_at": time.Now().UTC()}
	if id, ok := c.Get("audit_server_id"); ok {
		updates["server_id"] = id
		updates["target"] = "服务器 #" + strconv.FormatUint(uint64(id.(uint)), 10)
	}
	if raw, ok := c.Get("user"); ok {
		updates["user"] = raw.(models.User).Username
	}
	if a.DB.Model(&models.OperationLog{}).Where("id = ?", entry.ID).Updates(updates).Error != nil { /* requested entry remains for forensic review */
	}
}
