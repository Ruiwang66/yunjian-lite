package models

import "time"

type Website struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Name        string    `json:"name"`
	Domain      string    `gorm:"uniqueIndex:idx_website" json:"domain"`
	RootPath    string    `json:"root_path"`
	ServerID    uint      `gorm:"uniqueIndex:idx_website;index" json:"server_id"`
	NginxConfig string    `json:"nginx_config"`
	PHPVersion  string    `json:"php_version"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
type SSLCertificate struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	WebsiteID  uint      `gorm:"uniqueIndex" json:"website_id"`
	ServerID   uint      `gorm:"index" json:"server_id"`
	Domain     string    `json:"domain"`
	Status     string    `json:"status"`
	Issuer     string    `json:"issuer"`
	NotBefore  int64     `json:"not_before"`
	NotAfter   int64     `json:"not_after"`
	DaysLeft   int       `json:"days_left"`
	TLSVersion string    `json:"tls_version"`
	ChainJSON  string    `json:"chain_json"`
	Warning    string    `json:"warning"`
	Error      string    `json:"error"`
	UpdatedAt  time.Time `json:"updated_at"`
}
type Domain struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	WebsiteID uint      `gorm:"uniqueIndex" json:"website_id"`
	ServerID  uint      `gorm:"index" json:"server_id"`
	Domain    string    `json:"domain"`
	Status    string    `json:"status"`
	AJSON     string    `json:"a_json"`
	AAAAJSON  string    `json:"aaaa_json"`
	UpdatedAt time.Time `json:"updated_at"`
}
type BackupTask struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	WebsiteID uint      `gorm:"index" json:"website_id"`
	ServerID  uint      `gorm:"index" json:"server_id"`
	TaskID    string    `gorm:"uniqueIndex" json:"task_id"`
	Name      string    `json:"name"`
	Kind      string    `json:"kind"`
	Size      int64     `json:"size"`
	SHA256    string    `json:"sha256"`
	Status    string    `json:"status"`
	CreatedAt time.Time `gorm:"index" json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
type HealthCheck struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	WebsiteID    uint      `gorm:"uniqueIndex" json:"website_id"`
	ServerID     uint      `gorm:"index" json:"server_id"`
	Domain       string    `json:"domain"`
	Status       string    `json:"status"`
	HTTPStatus   int       `json:"http_status"`
	ResponseMS   int64     `json:"response_ms"`
	Accessible   bool      `json:"accessible"`
	Error        string    `json:"error"`
	WarningsJSON string    `json:"warnings_json"`
	UpdatedAt    time.Time `json:"updated_at"`
}
