// Package service contains business logic reusable by HTTP and future management modules.
package service

import (
	"cloudease-lite/internal/privatefs"
	"cloudease-lite/server/models"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"os"
	"path/filepath"
)

func RandomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// BootstrapAdmin only writes a password on initial creation or an explicit CLI reset.
// The database stores bcrypt; the initial password file is restricted to its owner.
func BootstrapAdmin(db *gorm.DB, dir string, reset bool) (bool, error) {
	var user models.User
	err := db.Where("username = ?", "admin").First(&user).Error
	if err == nil && !reset {
		return false, nil
	}
	if err != nil && err != gorm.ErrRecordNotFound {
		return false, err
	}
	password, err := RandomToken()
	if err != nil {
		return false, err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return false, err
	}
	passwordPath := filepath.Join(dir, "admin-password.txt")
	var previous []byte
	_, readErr := os.Lstat(passwordPath)
	if readErr == nil {
		if e := privatefs.File(passwordPath); e != nil {
			return false, e
		}
		previous, readErr = privatefs.ReadSecret(passwordPath)
	}
	if readErr != nil && !os.IsNotExist(readErr) {
		return false, readErr
	}
	if err := privatefs.AtomicWrite(passwordPath, []byte(fmt.Sprintf("username: admin\npassword: %s\n", password))); err != nil {
		return false, err
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		if user.ID == 0 {
			user = models.User{Username: "admin", Password: string(hash)}
			if err := tx.Create(&user).Error; err != nil {
				return err
			}
		} else {
			if err := tx.Model(&user).Update("password", string(hash)).Error; err != nil {
				return err
			}
		}
		return tx.Where("user_id = ?", user.ID).Delete(&models.Session{}).Error
	})
	if err != nil {
		if readErr == nil {
			_ = privatefs.AtomicWrite(passwordPath, previous)
		} else {
			_ = os.Remove(passwordPath)
		}
		return false, err
	}
	return true, nil
}
