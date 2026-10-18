package models

import "time"

type Role struct {
	Name        string `gorm:"primaryKey;size:16" json:"name"`
	Description string `json:"description"`
}
type DockerContainer struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	ServerID    uint      `gorm:"uniqueIndex:idx_container" json:"server_id"`
	ContainerID string    `gorm:"uniqueIndex:idx_container" json:"container_id"`
	Name        string    `json:"name"`
	State       string    `json:"state"`
	Ports       string    `json:"ports"`
	CreatedAt   string    `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
type Service struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	ServerID  uint      `gorm:"uniqueIndex:idx_service" json:"server_id"`
	Name      string    `gorm:"uniqueIndex:idx_service" json:"name"`
	State     string    `json:"state"`
	Enabled   string    `json:"enabled"`
	Allowed   bool      `json:"allowed"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Files stores metadata only. File contents never enter this table or operation logs.
type File struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	ServerID   uint      `gorm:"uniqueIndex:idx_file" json:"server_id"`
	Root       int       `gorm:"uniqueIndex:idx_file" json:"root"`
	Path       string    `gorm:"uniqueIndex:idx_file" json:"path"`
	Size       int64     `json:"size"`
	Directory  bool      `json:"directory"`
	ModifiedAt int64     `json:"modified_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}
type OperationLog struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	ServerID  uint      `gorm:"index" json:"server_id"`
	User      string    `json:"user"`
	IP        string    `json:"ip"`
	Action    string    `json:"action"`
	Target    string    `json:"target"`
	Status    string    `json:"status"`
	CreatedAt time.Time `gorm:"index" json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
type ManagementTask struct {
	WebsiteID      uint      `gorm:"index" json:"website_id,omitempty"`
	ID             string    `gorm:"primaryKey;size:43" json:"id"`
	ServerID       uint      `gorm:"index" json:"server_id"`
	UserID         uint      `json:"-"`
	SessionID      uint      `json:"-"`
	OperationLogID uint      `json:"-"`
	KeyHash        string    `json:"-"`
	LeaseHash      string    `json:"-"`
	Action         string    `json:"action"`
	RequestJSON    string    `gorm:"type:text" json:"-"`
	ResultJSON     string    `gorm:"type:text" json:"-"`
	Status         string    `gorm:"index" json:"status"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	ExpiresAt      time.Time `gorm:"index" json:"expires_at"`
}
