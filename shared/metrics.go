// Package shared defines the wire protocol shared by the server and Agent.
package shared

// Report contains one sample. Byte counters are cumulative since boot; rates are bytes/second.
type Report struct {
	SentAt          int64   `json:"sent_at,omitempty"`
	Nonce           string  `json:"nonce,omitempty"`
	Name            string  `json:"name"`
	IP              string  `json:"ip"`
	OS              string  `json:"os"`
	Platform        string  `json:"platform"`
	PlatformVersion string  `json:"platform_version"`
	Arch            string  `json:"arch"`
	CPUCores        int     `json:"cpu_cores"`
	CPU             float64 `json:"cpu"`
	Memory          float64 `json:"memory"`
	MemoryTotal     uint64  `json:"memory_total"`
	MemoryUsed      uint64  `json:"memory_used"`
	Disk            float64 `json:"disk"`
	DiskTotal       uint64  `json:"disk_total"`
	DiskUsed        uint64  `json:"disk_used"`
	NetworkRX       uint64  `json:"network_rx"`
	NetworkTX       uint64  `json:"network_tx"`
	NetworkRXRate   float64 `json:"network_rx_rate"`
	NetworkTXRate   float64 `json:"network_tx_rate"`
	Uptime          uint64  `json:"uptime"`
}
