package api

import (
	"cloudease-lite/server/models"
	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"regexp"
	"strings"
)

var usernamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`)

func (a *API) users(c *gin.Context) {
	items := []models.User{}
	if a.DB.Order("id").Limit(100).Find(&items).Error != nil {
		dbError(c)
		return
	}
	c.JSON(200, items)
}
func (a *API) roles(c *gin.Context) {
	items := []models.Role{}
	if a.DB.Order("name").Find(&items).Error != nil {
		dbError(c)
		return
	}
	c.JSON(200, items)
}
func (a *API) createUser(c *gin.Context) {
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}
	if !decodeJSON(c, &in) {
		return
	}
	if !usernamePattern.MatchString(in.Username) || !validRole(in.Role) || len(in.Password) < 12 || len(in.Password) > 72 || strings.TrimSpace(in.Password) != in.Password {
		fail(c, 400, "账号、角色或密码无效（密码 12–72 字节）")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		fail(c, 500, "创建密码失败")
		return
	}
	u := models.User{Username: in.Username, Password: string(hash), Role: in.Role}
	err = a.DB.Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&models.User{}).Count(&count).Error; err != nil {
			return err
		}
		if count >= 100 {
			return gorm.ErrInvalidData
		}
		return tx.Create(&u).Error
	})
	if err != nil {
		fail(c, 409, "账号已存在或用户数量达到限制")
		return
	}
	c.JSON(201, u)
}
func (a *API) updateUser(c *gin.Context) {
	var in struct {
		Role     string `json:"role"`
		Password string `json:"password"`
	}
	if !decodeJSON(c, &in) {
		return
	}
	if !validRole(in.Role) || (in.Password != "" && (len(in.Password) < 12 || len(in.Password) > 72 || strings.TrimSpace(in.Password) != in.Password)) {
		fail(c, 400, "角色或密码无效")
		return
	}
	updates := map[string]any{"role": in.Role}
	if in.Password != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
		if err != nil {
			fail(c, 500, "密码更新失败")
			return
		}
		updates["password"] = string(hash)
	}
	err := a.DB.Transaction(func(tx *gorm.DB) error {
		var u models.User
		if err := tx.First(&u, "id = ?", c.Param("id")).Error; err != nil {
			return err
		}
		if u.Role == "admin" && in.Role != "admin" {
			var admins int64
			if err := tx.Model(&models.User{}).Where("role = ?", "admin").Count(&admins).Error; err != nil {
				return err
			}
			if admins <= 1 {
				return gorm.ErrInvalidData
			}
		}
		if err := tx.Model(&u).Updates(updates).Error; err != nil {
			return err
		}
		return tx.Where("user_id = ?", u.ID).Delete(&models.Session{}).Error
	})
	if err != nil {
		fail(c, 409, "用户不存在或不能降级最后一位管理员")
		return
	}
	c.JSON(200, gin.H{"ok": true})
}
func (a *API) deleteUser(c *gin.Context) {
	actor := c.MustGet("user").(models.User)
	err := a.DB.Transaction(func(tx *gorm.DB) error {
		var u models.User
		if err := tx.First(&u, "id = ?", c.Param("id")).Error; err != nil {
			return err
		}
		if u.ID == actor.ID {
			return gorm.ErrInvalidData
		}
		if u.Role == "admin" {
			var admins int64
			if err := tx.Model(&models.User{}).Where("role = ?", "admin").Count(&admins).Error; err != nil {
				return err
			}
			if admins <= 1 {
				return gorm.ErrInvalidData
			}
		}
		if err := tx.Where("user_id = ?", u.ID).Delete(&models.Session{}).Error; err != nil {
			return err
		}
		return tx.Delete(&u).Error
	})
	if err != nil {
		fail(c, 409, "不能删除当前用户、最后一位管理员或不存在的用户")
		return
	}
	c.JSON(200, gin.H{"ok": true})
}
