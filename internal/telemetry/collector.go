package telemetry

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// clockFloor is the oldest believable wall-clock value. This device shipped in
// 2016 and has been observed reporting 2019 long after that, so any timestamp
// older than this floor is flagged rather than trusted.
var clockFloor = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

// Sampler owns all collection state. It is safe for concurrent use; callers
// normally drive it from a single background ticker and read the cached result.
type Sampler struct {
	API      *API
	ProcRoot string
	SysRoot  string
	Mounts   []string
	// Prime is how long to sample before the first CPU/network rate can be
	// computed. Small enough to stay invisible, large enough to be meaningful.
	Prime time.Duration

	mu         sync.Mutex
	prevCPU    *cpuTimes
	prevNet    map[string]netCounters
	lastSample time.Time
}

// NewSampler returns a sampler with paths matching the router's filesystem.
func NewSampler(api *API) *Sampler {
	return &Sampler{
		API:      api,
		ProcRoot: "/proc",
		SysRoot:  "/sys",
		Mounts:   []string{"/", "/usr", "/cache", "/usr/zte_web", "/firmware"},
		Prime:    250 * time.Millisecond,
	}
}

// Snapshot collects one observation. It performs only reads.
func (s *Sampler) Snapshot(ctx context.Context) Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	snap := Snapshot{GeneratedAt: now}
	snap.Device.ClockTrusted = now.After(clockFloor)

	if up, err := readUptime(filepath.Join(s.ProcRoot, "uptime")); err == nil {
		snap.Device.UptimeSeconds = up
	} else {
		snap.Warnings = append(snap.Warnings, "uptime: "+err.Error())
	}

	s.collectRates(&snap)

	if mem, err := readMeminfo(filepath.Join(s.ProcRoot, "meminfo")); err == nil {
		snap.System.MemTotalKB = mem["MemTotal"]
		avail := mem["MemAvailable"]
		if avail == 0 {
			// Kernel 3.10 predates MemAvailable on some vendor trees.
			avail = mem["MemFree"] + mem["Buffers"] + mem["Cached"]
		}
		snap.System.MemAvailableKB = avail
		if mem["MemTotal"] > 0 {
			used := 100 * float64(mem["MemTotal"]-avail) / float64(mem["MemTotal"])
			snap.System.MemUsedPercent = &used
		}
	} else {
		snap.Warnings = append(snap.Warnings, "meminfo: "+err.Error())
	}

	if load, err := readLoad(filepath.Join(s.ProcRoot, "loadavg")); err == nil {
		snap.System.Load1, snap.System.Load5, snap.System.Load15 = load[0], load[1], load[2]
	} else {
		snap.Warnings = append(snap.Warnings, "loadavg: "+err.Error())
	}

	snap.System.Disks = collectDisks(s.Mounts, &snap.Warnings)

	if zones, warnings := collectThermal(s.SysRoot); len(warnings) > 0 || len(zones) > 0 {
		snap.Thermal = zones
		snap.Warnings = append(snap.Warnings, warnings...)
	}
	power, warnings := collectPower(s.SysRoot)
	snap.Warnings = append(snap.Warnings, warnings...)
	snap.Power.Status = power.Status
	snap.Power.Health = power.Health
	snap.Power.VoltageV = power.VoltageV
	snap.Power.CurrentMA = power.CurrentMA
	snap.Power.TempC = power.TempC
	snap.Power.ChargerPresent = power.ChargerPresent
	snap.Power.USBOnline = power.USBOnline
	snap.Power.BatteryPercent = power.BatteryPercent

	s.collectVendor(ctx, &snap)
	return snap
}

func (s *Sampler) collectRates(snap *Snapshot) {
	var cpuPrev *cpuTimes
	if s.prevCPU != nil {
		p := *s.prevCPU
		cpuPrev = &p
	}
	netPrev := s.prevNet
	start := s.lastSample

	cpuNow, cpuErr := readCPU(filepath.Join(s.ProcRoot, "stat"))
	netNow, netErr := readNetDev(filepath.Join(s.ProcRoot, "net", "dev"))

	// First run: take a short second sample so the dashboard does not open with
	// empty gauges.
	if (cpuErr == nil && cpuPrev == nil) || (netErr == nil && netPrev == nil) {
		primeStart := time.Now()
		time.Sleep(s.Prime)
		if cpuErr == nil {
			cpuPrev = &cpuNow
			cpuNow, cpuErr = readCPU(filepath.Join(s.ProcRoot, "stat"))
		}
		if netErr == nil {
			netPrev = netNow
			netNow, netErr = readNetDev(filepath.Join(s.ProcRoot, "net", "dev"))
		}
		start = primeStart
	}

	elapsed := 0.0
	if !start.IsZero() {
		elapsed = time.Since(start).Seconds()
	}
	s.lastSample = time.Now()

	if cpuErr != nil {
		snap.Warnings = append(snap.Warnings, "stat: "+cpuErr.Error())
	} else {
		s.prevCPU = &cpuNow
		if cpuPrev != nil {
			snap.System.CPUPercent = cpuPercent(*cpuPrev, cpuNow)
		}
	}

	if netErr != nil {
		snap.Warnings = append(snap.Warnings, "net/dev: "+netErr.Error())
		return
	}
	s.prevNet = netNow
	if netPrev == nil || elapsed <= 0.05 {
		return
	}
	for name, cur := range netNow {
		prev, ok := netPrev[name]
		if !ok {
			continue
		}
		rx := float64(delta32(cur.rx, prev.rx)) * 8 / elapsed
		tx := float64(delta32(cur.tx, prev.tx)) * 8 / elapsed
		if strings.HasPrefix(name, "lo") && rx == 0 && tx == 0 {
			continue
		}
		snap.System.Interfaces = append(snap.System.Interfaces, Net{
			Name: name, RxBps: rx, TxBps: tx, RxBytes: cur.rx, TxBytes: cur.tx,
		})
	}
}

func (s *Sampler) collectVendor(ctx context.Context, snap *Snapshot) {
	if s.API == nil {
		return
	}
	values, err := s.API.Fetch(ctx, allowlist)
	if err != nil {
		snap.Warnings = append(snap.Warnings, "vendor api: "+err.Error())
		return
	}

	snap.Device.Firmware = str(values, "wa_inner_version")
	snap.Device.Hardware = str(values, "hardware_version")

	snap.Cell.NetworkType = str(values, "network_type")
	snap.Cell.PPPStatus = str(values, "ppp_status")
	snap.Cell.ModemState = str(values, "modem_main_state")
	snap.Cell.Service = snap.Cell.PPPStatus == "ppp_connected"
	snap.Cell.RSSI = intPtr(values, "rssi")
	snap.Cell.RSCP = intPtr(values, "rscp")
	snap.Cell.RSRP = intPtr(values, "lte_rsrp")
	snap.Cell.SignalBar = intPtr(values, "signalbar")
	snap.Cell.WANConnected = bool01Ptr(values, "wan_connect_status")

	// The API figure is a percentage and matches sysfs capacity on this device.
	if p := intPtr(values, "battery_vol_percent"); p != nil {
		snap.Power.BatteryPercent = p
	}
	if charging := bool01Ptr(values, "battery_charging"); charging != nil {
		if *charging {
			snap.Power.Status = "Charging"
		} else if snap.Power.Status == "" {
			snap.Power.Status = "Discharging"
		}
	}

	snap.Usage.MonthRxBytes = int64Ptr(values, "monthly_rx_bytes")
	snap.Usage.MonthTxBytes = int64Ptr(values, "monthly_tx_bytes")
	snap.Usage.MonthTimeSeconds = int64Ptr(values, "monthly_time")
	snap.Usage.SessionTimeSecs = int64Ptr(values, "realtime_time")
	snap.Usage.SessionRxBps = floatPtr(values, "realtime_rx_thrpt")
	snap.Usage.SessionTxBps = floatPtr(values, "realtime_tx_thrpt")

	snap.WiFi.RadioOff = bool01Ptr(values, "RadioOff")
	snap.WiFi.Channel = intPtr(values, "Channel")
	snap.WiFi.MaxClients = intPtr(values, "MAX_Access_num")
	snap.WiFi.Band = str(values, "wifi_band")
	snap.WiFi.FiveGHz = bool01Ptr(values, "wifi_5g_enable")
	snap.WiFi.Clients = clientCount(values)
}
