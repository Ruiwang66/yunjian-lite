// Package models owns the persistent SQLite schema. Secret hashes are never serialized.
package models

import (
	"cloudease-lite/shared"
	"time"
)

type User struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Username  string    `gorm:"uniqueIndex;size:64" json:"username"`
	Password  string    `gorm:"not null" json:"-"`
	CreatedAt time.Time `json:"created_at"`
}
type Session struct {
	ID        uint      `gorm:"primaryKey"`
	TokenHash string    `gorm:"uniqueIndex;size:64"`
	UserID    uint      `gorm:"index"`
	ExpiresAt time.Time `gorm:"index"`
}
type Server struct {
	LastReportAt    int64      `json:"-"`
	LastReportNonce string     `json:"-"`
	ID              uint       `gorm:"primaryKey" json:"id"`
	Name            string     `json:"name"`
	Token           string     `gorm:"uniqueIndex;size:64" json:"-"`
	IP              string     `json:"ip"`
	Status          string     `gorm:"index" json:"status"`
	OS              string     `json:"os"`
	Platform        string     `json:"platform"`
	PlatformVersion string     `json:"platform_version"`
	Arch            string     `json:"arch"`
	CPUCores        int        `json:"cpu_cores"`
	Uptime          uint64     `json:"uptime"`
	LastSeen        *time.Time `json:"last_seen"`
	CreatedAt       time.Time  `json:"created_at"`
}
type Metric struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	ServerID      uint      `gorm:"index:idx_server_time,priority:1" json:"server_id"`
	CPU           float64   `json:"cpu"`
	Memory        float64   `json:"memory"`
	Disk          float64   `json:"disk"`
	MemoryTotal   uint64    `json:"memory_total"`
	MemoryUsed    uint64    `json:"memory_used"`
	DiskTotal     uint64    `json:"disk_total"`
	DiskUsed      uint64    `json:"disk_used"`
	NetworkRX     uint64    `json:"network_rx"`
	NetworkTX     uint64    `json:"network_tx"`
	NetworkRXRate float64   `json:"network_rx_rate"`
	NetworkTXRate float64   `json:"network_tx_rate"`
	Uptime        uint64    `json:"uptime"`
	CreatedAt     time.Time `gorm:"index:idx_server_time,priority:2;index" json:"created_at"`
}

func Sample(id uint, r shared.Report, now time.Time) Metric {
	return Metric{ServerID: id, CPU: r.CPU, Memory: r.Memory, Disk: r.Disk, MemoryTotal: r.MemoryTotal, MemoryUsed: r.MemoryUsed, DiskTotal: r.DiskTotal, DiskUsed: r.DiskUsed, NetworkRX: r.NetworkRX, NetworkTX: r.NetworkTX, NetworkRXRate: r.NetworkRXRate, NetworkTXRate: r.NetworkTXRate, Uptime: r.Uptime, CreatedAt: now}
}

type SystemLog struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	ServerID  uint      `gorm:"index" json:"server_id"`
	Level     string    `json:"level"`
	Message   string    `json:"message"`
	CreatedAt time.Time `gorm:"index" json:"created_at"`
}
