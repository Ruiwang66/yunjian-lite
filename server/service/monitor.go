package service

import (
	"cloudease-lite/server/models"
	"context"
	"gorm.io/gorm"
	"log"
	"time"
)

// Monitor records offline transitions and bounds metric/log/session storage.
func Monitor(ctx context.Context, db *gorm.DB, offline time.Duration, retentionDays int, broadcast func(string)) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	lastCleanup := time.Time{}
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			now = now.UTC() // SQLite compares timestamp strings; query bounds must use UTC.
			changed := false
			err := db.Transaction(func(tx *gorm.DB) error {
				items := []models.Server{}
				if err := tx.Where("status = ? AND last_seen < ?", "online", now.Add(-offline)).Find(&items).Error; err != nil {
					return err
				}
				for _, s := range items {
					if err := tx.Model(&s).Update("status", "offline").Error; err != nil {
						return err
					}
					if err := tx.Create(&models.SystemLog{ServerID: s.ID, Level: "WARN", Message: "Agent disconnected: no report for 90 seconds", CreatedAt: now.UTC()}).Error; err != nil {
						return err
					}
					changed = true
				}
				return nil
			})
			if err != nil {
				log.Printf("offline monitor: %v", err)
			} else if changed {
				broadcast("status_changed")
			}
			if now.Sub(lastCleanup) > time.Hour {
				cutoff := now.Add(-time.Duration(retentionDays) * 24 * time.Hour)
				for _, entry := range []struct {
					Model  any
					Column string
					Before time.Time
				}{{&models.Metric{}, "created_at", cutoff}, {&models.SystemLog{}, "created_at", cutoff}, {&models.Session{}, "expires_at", now}} {
					if err := db.Where(entry.Column+" < ?", entry.Before).Delete(entry.Model).Error; err != nil {
						log.Printf("retention cleanup: %v", err)
					}
				}
				lastCleanup = now
			}
		}
	}
}
