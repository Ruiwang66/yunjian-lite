package service

import (
	"cloudease-lite/server/models"
	"context"
	"fmt"
	"gorm.io/gorm"
	"time"
)

// Each statement commits at most 500 deletions, releasing the single SQLite
// connection between batches. Predicates come only from internal callers.
func deleteBatches(db *gorm.DB, model any, predicate string, args ...any) error {
	key := "id"
	if _, ok := model.(*models.SecurityAlertRead); ok {
		key = "rowid"
	} // Composite primary key.
	for {
		if err := db.Statement.Context.Err(); err != nil {
			return err
		}
		ids := db.Model(model).Select(key).Where(predicate, args...).Order(key).Limit(500)
		result := db.Where(key+" IN (?)", ids).Delete(model)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected < 500 {
			return nil
		}
	}
}

func CleanupMonitoring(ctx context.Context, db *gorm.DB, now time.Time, retentionDays int) error {
	if retentionDays < 1 || retentionDays > 365 {
		return fmt.Errorf("invalid monitoring retention period")
	}
	now = now.UTC()
	db = db.WithContext(ctx)
	cutoff := now.Add(-time.Duration(retentionDays) * 24 * time.Hour)
	for _, model := range []any{&models.Metric{}, &models.SystemLog{}} {
		if err := deleteBatches(db, model, "created_at < ?", cutoff); err != nil {
			return err
		}
	}
	return deleteBatches(db, &models.Session{}, "expires_at < ?", now)
}
