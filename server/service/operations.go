package service

import (
	"cloudease-lite/server/models"
	"gorm.io/gorm"
	"time"
)

func ExpireOperations(db *gorm.DB, now time.Time) error {
	now = now.UTC()
	err := db.Transaction(func(tx *gorm.DB) error {
		var tasks []models.ManagementTask
		if err := tx.Where("status IN ? AND expires_at <= ?", []string{"queued", "running"}, now).Order("expires_at").Limit(100).Find(&tasks).Error; err != nil {
			return err
		}
		for _, task := range tasks {
			if err := FailSecurityScan(tx, task, "expired", "任务超时或客户端离线", now); err != nil {
				return err
			}
			if err := tx.Model(&models.BackupTask{}).Where("task_id = ?", task.ID).Updates(map[string]any{"status": "expired", "updated_at": now}).Error; err != nil {
				return err
			}
			if err := tx.Model(&task).Updates(map[string]any{"status": "expired", "request_json": "", "updated_at": now}).Error; err != nil {
				return err
			}
			if err := tx.Model(&models.OperationLog{}).Where("id = ?", task.OperationLogID).Updates(map[string]any{"status": "expired", "updated_at": now}).Error; err != nil {
				return err
			}
		}
		if err := tx.Model(&models.BackupTask{}).Where("status IN ? AND task_id IN (?)", []string{"queued", "running"}, tx.Model(&models.ManagementTask{}).Select("id").Where("status IN ?", []string{"running", "success", "failed", "cancelled", "expired"})).Updates(map[string]any{"status": gorm.Expr("(SELECT status FROM management_tasks WHERE management_tasks.id = backup_tasks.task_id)"), "updated_at": now}).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}
	// Clear temporary payloads in short commits without rewriting completion
	// timestamps or repeatedly updating already-empty rows on every monitor tick.
	for {
		ids := db.Model(&models.ManagementTask{}).Select("id").Where("updated_at < ? AND status NOT IN ? AND (request_json <> '' OR result_json <> '')", now.Add(-5*time.Minute), []string{"queued", "running"}).Order("id").Limit(500)
		result := db.Model(&models.ManagementTask{}).Where("id IN (?)", ids).UpdateColumns(map[string]any{"request_json": "", "result_json": ""})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected < 500 {
			break
		}
	}
	if err := deleteBatches(db, &models.ManagementTask{}, "created_at < ?", now.Add(-24*time.Hour)); err != nil {
		return err
	}
	return deleteBatches(db, &models.OperationLog{}, "created_at < ?", now.Add(-90*24*time.Hour))
}
