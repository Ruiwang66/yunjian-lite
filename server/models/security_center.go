package models

import "time"

type SecurityScan struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	ServerID    uint      `gorm:"index" json:"server_id"`
	TaskID      string    `gorm:"uniqueIndex" json:"task_id"`
	Status      string    `gorm:"index" json:"status"`
	Error       string    `json:"error"`
	ReportJSON  string    `gorm:"type:text" json:"-"`
	OptionsJSON string    `gorm:"type:text" json:"-"`
	CreatedAt   time.Time `gorm:"index" json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
type SecurityFinding struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	ServerID   uint      `gorm:"index" json:"server_id"`
	ScanID     uint      `gorm:"index" json:"scan_id"`
	Rule       string    `json:"rule"`
	Dimension  string    `json:"dimension"`
	Title      string    `json:"title"`
	Severity   string    `gorm:"index" json:"severity"`
	Status     string    `json:"status"`
	Target     string    `json:"target"`
	Evidence   string    `json:"evidence"`
	Reason     string    `json:"reason"`
	Advice     string    `json:"advice"`
	Workflow   string    `json:"workflow"`
	ReviewedBy string    `json:"reviewed_by"`
	CreatedAt  time.Time `gorm:"index" json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}
type SecurityEvent struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	ServerID   uint      `gorm:"uniqueIndex:idx_security_event;index" json:"server_id"`
	Key        string    `gorm:"uniqueIndex:idx_security_event" json:"-"`
	Rule       string    `json:"rule"`
	Severity   string    `gorm:"index" json:"severity"`
	Message    string    `json:"message"`
	ObservedAt time.Time `json:"observed_at"`
	CreatedAt  time.Time `gorm:"index" json:"created_at"`
}
type SecurityAlert struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	ServerID  uint      `gorm:"index" json:"server_id"`
	Key       string    `gorm:"uniqueIndex" json:"-"`
	Severity  string    `gorm:"index" json:"severity"`
	Message   string    `json:"message"`
	CreatedAt time.Time `gorm:"index" json:"created_at"`
}

// Read state is per user, so one operator cannot mark another user's alerts read.
type SecurityAlertRead struct {
	AlertID uint      `gorm:"primaryKey" json:"alert_id"`
	UserID  uint      `gorm:"primaryKey" json:"-"`
	ReadAt  time.Time `json:"read_at"`
}
type SecurityScore struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	ServerID  uint      `gorm:"index" json:"server_id"`
	ScanID    uint      `gorm:"uniqueIndex" json:"scan_id"`
	Score     *int      `json:"score"`
	Risk      string    `json:"risk"`
	Delta     *int      `json:"delta"`
	GradeJSON string    `gorm:"type:text" json:"grade_json"`
	CreatedAt time.Time `gorm:"index" json:"created_at"`
}
type SecurityRule struct {
	ServerID      uint      `gorm:"primaryKey" json:"server_id"`
	OptionsJSON   string    `gorm:"type:text" json:"options_json"`
	RetentionDays int       `json:"retention_days"`
	MuteUntil     time.Time `json:"mute_until"`
	IntervalHours int       `json:"interval_hours"`
	NextScanAt    time.Time `json:"next_scan_at"`
	UserID        uint      `json:"-"`
	SessionID     uint      `json:"-"`
	UpdatedAt     time.Time `json:"updated_at"`
}
