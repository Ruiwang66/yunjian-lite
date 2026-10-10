package api

import (
	"cloudease-lite/server/models"
	"cloudease-lite/server/service"
	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (a *API) login(c *gin.Context) {
	if a.loginSlots != nil {
		select {
		case a.loginSlots <- struct{}{}:
			defer func() { <-a.loginSlots }()
		default:
			fail(c, 429, "服务繁忙，请稍后重试")
			return
		}
	}
	if !a.allowLogin(c.ClientIP()) {
		fail(c, 429, "登录尝试过多，请 15 分钟后再试")
		return
	}
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !decodeJSON(c, &in) {
		return
	}
	if len(in.Username) == 0 || len(in.Password) == 0 || len(in.Username) > 64 || len(in.Password) > 72 {
		fail(c, 400, "请输入有效的账号和密码")
		return
	}
	var u models.User
	err := a.DB.Where("username = ?", in.Username).First(&u).Error
	// Also perform bcrypt work for unknown usernames to reduce timing disclosure.
	hash := u.Password
	if hash == "" {
		hash = "$2a$10$7EqJtq98hPqEX7fNZaFWoO5Y6.4D4S.3LQHDQcTUtQgAbYvMrFPe6"
	}
	match := bcrypt.CompareHashAndPassword([]byte(hash), []byte(in.Password)) == nil
	if err != nil || !match {
		fail(c, 401, "账号或密码错误")
		return
	}
	raw, err := service.RandomToken()
	if err != nil {
		fail(c, 500, "创建会话失败")
		return
	}
	s := models.Session{TokenHash: service.HashToken(raw), UserID: u.ID, ExpiresAt: time.Now().UTC().Add(a.Config.SessionTTL)}
	if a.DB.Create(&s).Error != nil {
		dbError(c)
		return
	}
	a.loginMu.Lock()
	delete(a.attempts, c.ClientIP())
	a.loginMu.Unlock()
	a.cookie(c, raw, int(a.Config.SessionTTL.Seconds()))
	c.JSON(200, gin.H{"user": u})
}
func (a *API) me(c *gin.Context) {
	var u models.User
	if a.DB.First(&u, c.MustGet("session").(models.Session).UserID).Error != nil {
		dbError(c)
		return
	}
	c.JSON(200, u)
}
func (a *API) logout(c *gin.Context) {
	if a.DB.Delete(&models.Session{}, c.MustGet("session").(models.Session).ID).Error != nil {
		dbError(c)
		return
	}
	a.cookie(c, "", -1)
	c.JSON(200, gin.H{"ok": true})
}
func (a *API) password(c *gin.Context) {
	var in struct {
		Current string `json:"current_password"`
		Next    string `json:"new_password"`
	}
	if !decodeJSON(c, &in) {
		return
	}
	if len(in.Next) < 12 || len(in.Next) > 72 || len(in.Current) > 72 || strings.TrimSpace(in.Next) != in.Next {
		fail(c, 400, "新密码须为 12–72 字节，且首尾不能有空格")
		return
	}
	var u models.User
	if a.DB.First(&u, c.MustGet("session").(models.Session).UserID).Error != nil {
		dbError(c)
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(in.Current)) != nil {
		fail(c, 401, "当前密码错误")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(in.Next), bcrypt.DefaultCost)
	if err != nil {
		fail(c, 500, "密码更新失败")
		return
	}
	err = a.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&u).Update("password", string(hash)).Error; err != nil {
			return err
		}
		return tx.Where("user_id = ?", u.ID).Delete(&models.Session{}).Error
	})
	if err != nil {
		dbError(c)
		return
	}
	_ = os.Remove(filepath.Join(a.Config.DataDir, "admin-password.txt"))
	a.cookie(c, "", -1)
	c.JSON(200, gin.H{"ok": true})
}
