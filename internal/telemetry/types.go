// Package telemetry collects read-only statistics from a ZTE MF971V.
//
// Every collector here is intentionally read-only: it opens files under /proc
// and /sys, calls the on-device status API, and nothing else. There is no code
// path that writes to the device, sends a command that changes state, or
// forwards caller-supplied parameters to the vendor API.
package telemetry

import "time"

// Snapshot is one complete observation set. Pointer fields are used where the
// value may genuinely be unknown: a nil must never be rendered as zero.
type Snapshot struct {
	GeneratedAt time.Time `json:"generated_at"`
	Device      Device    `json:"device"`
	System      System    `json:"system"`
	Power       Power     `json:"power"`
	Thermal     []Zone    `json:"thermal"`
	Cell        Cell      `json:"cell"`
	Usage       Usage     `json:"usage"`
	WiFi        WiFi      `json:"wifi"`
	// Warnings lists collector problems with the field name that was lost.
	Warnings []string `json:"warnings,omitempty"`
}

// Device holds build strings and uptime.
type Device struct {
	Firmware      string  `json:"firmware,omitempty"`
	Hardware      string  `json:"hardware,omitempty"`
	UptimeSeconds float64 `json:"uptime_seconds"`
	// ClockTrusted is false when the system clock is implausible. Callers must
	// not derive durations from wall-clock timestamps alone.
	ClockTrusted bool `json:"clock_trusted"`
}

// System holds CPU, memory, load, storage and interface rates.
type System struct {
	CPUPercent     *float64 `json:"cpu_percent"`
	Load1          float64  `json:"load1"`
	Load5          float64  `json:"load5"`
	Load15         float64  `json:"load15"`
	MemTotalKB     int64    `json:"mem_total_kb"`
	MemAvailableKB int64    `json:"mem_available_kb"`
	MemUsedPercent *float64 `json:"mem_used_percent"`
	Disks          []Disk   `json:"disks,omitempty"`
	Interfaces     []Net    `json:"interfaces,omitempty"`
}

// Disk is one mounted filesystem.
type Disk struct {
	Mount   string `json:"mount"`
	TotalKB int64  `json:"total_kb"`
	FreeKB  int64  `json:"free_kb"`
	UsedPct *int   `json:"used_percent"`
}

// Net is a per-interface rate derived from two /proc/net/dev samples.
type Net struct {
	Name    string  `json:"name"`
	RxBps   float64 `json:"rx_bps"`
	TxBps   float64 `json:"tx_bps"`
	RxBytes uint64  `json:"rx_bytes"`
	TxBytes uint64  `json:"tx_bytes"`
}

// Power holds battery and charger state. CurrentMA is signed: negative means
// the battery is discharging.
type Power struct {
	BatteryPercent *int     `json:"battery_percent"`
	Status         string   `json:"status,omitempty"`
	Health         string   `json:"health,omitempty"`
	VoltageV       *float64 `json:"voltage_v"`
	CurrentMA      *float64 `json:"current_ma"`
	TempC          *float64 `json:"temp_c"`
	ChargerPresent *bool    `json:"charger_present"`
	USBOnline      *bool    `json:"usb_online"`
}

// Zone is one thermal sensor.
type Zone struct {
	Index int      `json:"index"`
	Type  string   `json:"type"`
	C     *float64 `json:"celsius"`
	Raw   *int64   `json:"raw"`
	// ScaleAssumed is true when the raw value had to be divided to reach a
	// plausible temperature. The MF971V reports mixed scales per zone.
	ScaleAssumed bool `json:"scale_assumed"`
}

// Cell holds connection state. Radio figures are nil when the modem reports
// nothing, which is common with no SIM or no service; they are never cached
// from a previous sample.
type Cell struct {
	NetworkType  string `json:"network_type,omitempty"`
	PPPStatus    string `json:"ppp_status,omitempty"`
	ModemState   string `json:"modem_state,omitempty"`
	Service      bool   `json:"service"`
	RSSI         *int   `json:"rssi_dbm"`
	RSCP         *int   `json:"rscp_dbm"`
	RSRP         *int   `json:"rsrp_dbm"`
	SignalBar    *int   `json:"signal_bar"`
	WANConnected *bool  `json:"wan_connected"`
}

// Usage holds vendor traffic counters.
type Usage struct {
	MonthRxBytes     *int64   `json:"month_rx_bytes"`
	MonthTxBytes     *int64   `json:"month_tx_bytes"`
	MonthTimeSeconds *int64   `json:"month_time_seconds"`
	SessionRxBps     *float64 `json:"session_rx_bps"`
	SessionTxBps     *float64 `json:"session_tx_bps"`
	SessionTimeSecs  *int64   `json:"session_time_seconds"`
}

// WiFi holds access-point state that the vendor API actually reports.
type WiFi struct {
	RadioOff   *bool  `json:"radio_off"`
	Channel    *int   `json:"channel"`
	MaxClients *int   `json:"max_clients"`
	Clients    *int   `json:"clients"`
	Band       string `json:"band,omitempty"`
	FiveGHz    *bool  `json:"five_ghz"`
}
