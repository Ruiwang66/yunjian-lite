package service

import (
	"cloudease-lite/server/models"
	"gorm.io/gorm"
	"time"
)

func ExpireOperations(db *gorm.DB, now time.Time) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var tasks []models.ManagementTask
		if err := tx.Where("status IN ? AND expires_at <= ?", []string{"queued", "running"}, now).Find(&tasks).Error; err != nil {
			return err
		}
		for _, task := range tasks {
			if err := tx.Model(&task).Updates(map[string]any{"status": "expired", "request_json": "", "updated_at": now}).Error; err != nil {
				return err
			}
			if err := tx.Model(&models.OperationLog{}).Where("id = ?", task.OperationLogID).Updates(map[string]any{"status": "expired", "updated_at": now}).Error; err != nil {
				return err
			}
		}
		// Results are available for five minutes. Completed tasks are removed after a day.
		if err := tx.Model(&models.ManagementTask{}).Where("updated_at < ?", now.Add(-5*time.Minute)).Updates(map[string]any{"request_json": "", "result_json": ""}).Error; err != nil {
			return err
		}
		if err := tx.Where("created_at < ?", now.Add(-24*time.Hour)).Delete(&models.ManagementTask{}).Error; err != nil {
			return err
		}
		return tx.Where("created_at < ?", now.Add(-90*24*time.Hour)).Delete(&models.OperationLog{}).Error
	})
}
