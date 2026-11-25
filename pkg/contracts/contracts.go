package contracts

import "time"

type FilesystemMetric struct {
	MountPoint  string  `json:"mount_point"`
	Device      string  `json:"device"`
	FSType      string  `json:"fs_type"`
	TotalBytes  uint64  `json:"total_bytes"`
	UsedBytes   uint64  `json:"used_bytes"`
	FreeBytes   uint64  `json:"free_bytes"`
	UsedPercent float64 `json:"used_percent"`
}

type NetworkInterfaceMetric struct {
	Name      string   `json:"name"`
	Addresses []string `json:"addresses,omitempty"`
	RxBytes   uint64   `json:"rx_bytes"`
	TxBytes   uint64   `json:"tx_bytes"`
	RxBps     float64  `json:"rx_bps"`
	TxBps     float64  `json:"tx_bps"`
}

type BatteryMetric struct {
	Percent float64 `json:"percent"`
	State   string  `json:"state"`
	Source  string  `json:"source,omitempty"`
}

type ProcessSummary struct {
	TotalCount int `json:"total_count"`
}

type OptionalMetrics struct {
	CPUTemperatureC *float64        `json:"cpu_temperature_c,omitempty"`
	Battery         *BatteryMetric  `json:"battery,omitempty"`
	Processes       *ProcessSummary `json:"processes,omitempty"`
}

type HostMetricPayload struct {
	RecordedAt      time.Time                `json:"recorded_at"`
	Hostname        string                   `json:"hostname"`
	OS              string                   `json:"os"`
	Platform        string                   `json:"platform"`
	PlatformVersion string                   `json:"platform_version"`
	Arch            string                   `json:"arch"`
	KernelVersion   string                   `json:"kernel_version"`
	IPAddress       string                   `json:"ip_address"`
	AgentVersion    string                   `json:"agent_version"`
	CPUPercent      float64                  `json:"cpu_percent"`
	CPUCores        int                      `json:"cpu_cores"`
	Load1m          float64                  `json:"load_1m"`
	Load5m          float64                  `json:"load_5m"`
	Load15m         float64                  `json:"load_15m"`
	MemTotalBytes   uint64                   `json:"mem_total_bytes"`
	MemUsedBytes    uint64                   `json:"mem_used_bytes"`
	MemAvailBytes   uint64                   `json:"mem_available_bytes"`
	MemPercent      float64                  `json:"mem_percent"`
	DiskTotalBytes  uint64                   `json:"disk_total_bytes"`
	DiskUsedBytes   uint64                   `json:"disk_used_bytes"`
	DiskPercent     float64                  `json:"disk_percent"`
	NetRxBytes      uint64                   `json:"net_rx_bytes"`
	NetTxBytes      uint64                   `json:"net_tx_bytes"`
	NetRxBps        float64                  `json:"net_rx_bps"`
	NetTxBps        float64                  `json:"net_tx_bps"`
	UptimeSeconds   uint64                   `json:"uptime_seconds"`
	Filesystems     []FilesystemMetric       `json:"filesystems"`
	Interfaces      []NetworkInterfaceMetric `json:"interfaces"`
	Optional        OptionalMetrics          `json:"optional"`
}

type AgentEnrollRequest struct {
	EnrollmentToken string   `json:"enrollment_token"`
	Hostname        string   `json:"hostname"`
	DisplayName     string   `json:"display_name,omitempty"`
	OS              string   `json:"os"`
	Arch            string   `json:"arch"`
	KernelVersion   string   `json:"kernel_version"`
	IPAddress       string   `json:"ip_address"`
	AgentVersion    string   `json:"agent_version"`
	Tags            []string `json:"tags,omitempty"`
}

type AgentEnrollResponse struct {
	AgentID        string `json:"agent_id"`
	HostID         string `json:"host_id"`
	OrganizationID string `json:"organization_id"`
	AgentToken     string `json:"agent_token"`
	TokenPrefix    string `json:"token_prefix"`
}
