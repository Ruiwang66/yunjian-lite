package service

import (
	"cloudease-lite/server/database"
	"cloudease-lite/server/models"
	"cloudease-lite/shared"
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestV05RetentionBoundaryAndCustomPeriod(t *testing.T) {
	for _, days := range []int{30, 45} {
		t.Run(time.Duration(days).String(), func(t *testing.T) {
			db, err := database.Open(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			conn, _ := db.DB()
			defer conn.Close()
			now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.FixedZone("CST", 8*3600))
			cutoff := now.UTC().Add(-time.Duration(days) * 24 * time.Hour)
			rows := make([]models.Metric, 1003)
			for i := range rows {
				rows[i] = models.Metric{CreatedAt: cutoff.Add(-time.Second)}
			}
			rows[1001].CreatedAt = cutoff
			rows[1002].CreatedAt = cutoff.Add(time.Second)
			if err := db.CreateInBatches(rows, 100).Error; err != nil {
				t.Fatal(err)
			}
			for _, at := range []time.Time{cutoff.Add(-time.Second), cutoff, cutoff.Add(time.Second)} {
				if err := db.Create(&models.SystemLog{CreatedAt: at}).Error; err != nil {
					t.Fatal(err)
				}
			}
			if err := db.Create(&models.OperationLog{CreatedAt: cutoff.Add(-time.Second)}).Error; err != nil {
				t.Fatal(err)
			}
			if err := CleanupMonitoring(context.Background(), db, now, days); err != nil {
				t.Fatal(err)
			}
			for _, model := range []any{&models.Metric{}, &models.SystemLog{}, &models.OperationLog{}} {
				want := int64(2)
				if _, ok := model.(*models.OperationLog); ok {
					want = 1
				}
				var count int64
				if err := db.Model(model).Count(&count).Error; err != nil || count != want {
					t.Fatalf("%T count=%d want=%d err=%v", model, count, want, err)
				}
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if err := CleanupMonitoring(ctx, db, now, days); err != context.Canceled {
				t.Fatalf("cancel=%v", err)
			}
		})
	}
}

func TestV05SecurityRetentionPreservesCustomPolicyAndReadState(t *testing.T) {
	db, err := database.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	conn, _ := db.DB()
	defer conn.Close()
	s := models.Server{Name: "fixture", Token: "fixture-hash"}
	if err := db.Create(&s).Error; err != nil {
		t.Fatal(err)
	}
	options, _ := json.Marshal(shared.DefaultSecurityOptions())
	policy := models.SecurityRule{ServerID: s.ID, RetentionDays: 120, OptionsJSON: string(options)}
	if err := db.Create(&policy).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.FixedZone("CST", 8*3600))
	cutoff := now.UTC().Add(-120 * 24 * time.Hour)
	for i, at := range []time.Time{cutoff.Add(-time.Second), cutoff, cutoff.Add(time.Second)} {
		alert := models.SecurityAlert{ServerID: s.ID, Key: time.Duration(i).String(), CreatedAt: at}
		if err := db.Create(&alert).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Create(&models.SecurityAlertRead{AlertID: alert.ID, UserID: 42, ReadAt: now.UTC()}).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Create(&models.SecurityScan{ServerID: s.ID, TaskID: time.Duration(i).String(), CreatedAt: at}).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Create(&models.Metric{ServerID: s.ID, CreatedAt: cutoff.Add(-time.Second)}).Error; err != nil {
		t.Fatal(err)
	}
	if err := CleanupSecurity(db, now); err != nil {
		t.Fatal(err)
	}
	for _, model := range []any{&models.SecurityAlert{}, &models.SecurityAlertRead{}, &models.SecurityScan{}} {
		var count int64
		if err := db.Model(model).Count(&count).Error; err != nil || count != 2 {
			t.Fatalf("%T count=%d err=%v", model, count, err)
		}
	}
	var unchanged models.SecurityRule
	if err := db.First(&unchanged, "server_id = ?", s.ID).Error; err != nil || unchanged.RetentionDays != 120 || unchanged.OptionsJSON != string(options) {
		t.Fatal("custom policy was reset", err)
	}
	var count int64
	if err := db.Model(&models.Metric{}).Count(&count).Error; err != nil || count != 1 {
		t.Fatal("security cleanup touched monitoring history")
	}
}

func TestV05ResultCleanupPreservesCompletionTimeAndBoundary(t *testing.T) {
	db, err := database.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	conn, _ := db.DB()
	defer conn.Close()
	now := time.Now().UTC()
	for _, id := range []string{"old", "boundary", "new"} {
		at := now.Add(-5 * time.Minute)
		if id == "old" {
			at = at.Add(-time.Second)
		} else if id == "new" {
			at = at.Add(time.Second)
		}
		row := models.ManagementTask{ID: id, Status: "success", RequestJSON: "request", ResultJSON: "result", CreatedAt: at, UpdatedAt: at}
		if err := db.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := ExpireOperations(db, now); err != nil {
		t.Fatal(err)
	}
	var rows []models.ManagementTask
	if err := db.Order("id").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row.ID == "old" {
			if row.ResultJSON != "" || !row.UpdatedAt.Equal(now.Add(-5*time.Minute-time.Second)) {
				t.Fatal("cleanup changed completion timestamp", row.ID)
			}
		} else if row.ResultJSON != "result" {
			t.Fatal("premature result cleanup", row.ID)
		}
	}
}
