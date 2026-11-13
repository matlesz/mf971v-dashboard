package telemetry

import (
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// scaleTemp converts a vendor thermal reading to Celsius.
//
// The MF971V mixes units per zone: SoC and PA sensors return plain degrees,
// while the battery zone returns a value two orders of magnitude larger than
// the same temperature reported elsewhere. There is no unit metadata in sysfs,
// so this applies a magnitude rule and reports whether it had to scale.
func scaleTemp(raw int64) (celsius float64, assumed bool) {
	abs := math.Abs(float64(raw))
	switch {
	case abs >= 1000:
		v := float64(raw) / 100
		if math.Abs(v) > 150 {
			return float64(raw) / 1000, true
		}
		return v, true
	case abs >= 200:
		return float64(raw) / 10, true
	default:
		return float64(raw), false
	}
}

// scaleVoltage converts power_supply voltage to volts. The kernel class
// documents microvolts, but this vendor driver reports millivolts (a 4.2 V
// battery appears as 4200), so the range decides.
func scaleVoltage(raw int64) (volts float64, ok bool) {
	switch {
	case raw >= 1_000_000:
		return float64(raw) / 1_000_000, true
	case raw >= 2_500 && raw <= 6_000:
		return float64(raw) / 1000, true
	default:
		return 0, false
	}
}

// scaleCurrent converts power_supply current to milliamps. As with voltage, the
// kernel class says microamps while this driver reports milliamps, and the sign
// is meaningful: negative is discharge.
func scaleCurrent(raw int64) (milliamps float64, ok bool) {
	if raw == 0 {
		return 0, true
	}
	abs := math.Abs(float64(raw))
	if abs <= 10_000 {
		return float64(raw), true
	}
	return float64(raw) / 1000, true
}

// scaleBatteryTemp converts the battery temperature field. On this device it is
// already Celsius and matches the thermal zone after that zone's scaling.
func scaleBatteryTemp(raw int64) (celsius float64, ok bool) {
	if raw > -40 && raw < 125 {
		return float64(raw), true
	}
	c, _ := scaleTemp(raw)
	return c, true
}

// collectThermal walks thermal_zone* directories under sysRoot.
func collectThermal(sysRoot string) ([]Zone, []string) {
	base := filepath.Join(sysRoot, "class", "thermal")
	entries, err := os.ReadDir(base)
	if err != nil {
		return nil, []string{"thermal: " + err.Error()}
	}

	var zones []Zone
	var warnings []string
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), "thermal_zone") {
			continue
		}
		index, err := strconv.Atoi(strings.TrimPrefix(entry.Name(), "thermal_zone"))
		if err != nil {
			continue
		}
		dir := filepath.Join(base, entry.Name())
		typ, err := readFileString(filepath.Join(dir, "type"))
		if err != nil {
			typ = "unknown"
		}
		raw, err := readFileInt(filepath.Join(dir, "temp"))
		if err != nil {
			warnings = append(warnings, "thermal zone "+entry.Name()+": "+err.Error())
			continue
		}
		c, assumed := scaleTemp(*raw)
		zones = append(zones, Zone{Index: index, Type: typ, C: &c, Raw: raw, ScaleAssumed: assumed})
	}
	return zones, warnings
}

type powerSupply struct {
	BatteryPercent *int
	Status         string
	Health         string
	VoltageV       *float64
	CurrentMA      *float64
	TempC          *float64
	ChargerPresent *bool
	USBOnline      *bool
}

// collectPower reads the standard power_supply class and applies the vendor's
// actual units, which differ from the kernel class documentation.
func collectPower(sysRoot string) (powerSupply, []string) {
	var p powerSupply
	var warnings []string
	base := filepath.Join(sysRoot, "class", "power_supply")
	battery := filepath.Join(base, "battery")

	if v, err := readFileInt(filepath.Join(battery, "capacity")); err == nil {
		percent := int(*v)
		p.BatteryPercent = &percent
	} else {
		warnings = append(warnings, "battery capacity: "+err.Error())
	}
	if s, err := readFileString(filepath.Join(battery, "status")); err == nil {
		p.Status = s
	}
	if s, err := readFileString(filepath.Join(battery, "health")); err == nil {
		p.Health = s
	}
	if v, err := readFileInt(filepath.Join(battery, "voltage_now")); err == nil {
		if volts, ok := scaleVoltage(*v); ok {
			p.VoltageV = &volts
		} else {
			warnings = append(warnings, "battery voltage: implausible raw value")
		}
	}
	if v, err := readFileInt(filepath.Join(battery, "current_avg")); err == nil {
		if ma, ok := scaleCurrent(*v); ok {
			p.CurrentMA = &ma
		}
	}
	if v, err := readFileInt(filepath.Join(battery, "temp")); err == nil {
		if c, ok := scaleBatteryTemp(*v); ok {
			p.TempC = &c
		}
	}
	if v, err := readFileInt(filepath.Join(base, "charger", "present")); err == nil {
		present := *v == 1
		p.ChargerPresent = &present
	}
	if v, err := readFileInt(filepath.Join(base, "usb", "online")); err == nil {
		online := *v == 1
		p.USBOnline = &online
	}
	return p, warnings
}
