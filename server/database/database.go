// Package database initializes SQLite with WAL and a single writer connection.
package database

import (
	"cloudease-lite/internal/privatefs"
	"cloudease-lite/server/models"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"log"
	"os"
	"path/filepath"
)

func Open(dir string) (*gorm.DB, error) {
	if err := privatefs.Directory(dir); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "cloudease.db")
	if _, err := os.Lstat(path); os.IsNotExist(err) {
		if err := privatefs.AtomicWrite(path, nil); err != nil {
			return nil, err
		}
	}
	if err := privatefs.File(path); err != nil {
		return nil, err
	}
	db, err := gorm.Open(sqlite.Open(filepath.Join(dir, "cloudease.db")), &gorm.Config{Logger: logger.New(log.New(os.Stderr, "database: ", log.LstdFlags), logger.Config{LogLevel: logger.Error, IgnoreRecordNotFoundError: true, ParameterizedQueries: true})})
	if err != nil {
		return nil, err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(1)
	for _, query := range []string{"PRAGMA journal_mode=WAL", "PRAGMA busy_timeout=5000", "PRAGMA foreign_keys=ON"} {
		if err := db.Exec(query).Error; err != nil {
			return nil, err
		}
	}
	if err := db.AutoMigrate(&models.User{}, &models.Session{}, &models.Server{}, &models.Metric{}, &models.SystemLog{}); err != nil {
		return nil, err
	}
	if err := privatefs.File(path); err != nil {
		return nil, err
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		if _, err := os.Lstat(path + suffix); err == nil {
			if err := privatefs.File(path + suffix); err != nil {
				return nil, err
			}
		}
	}
	return db, nil
}
