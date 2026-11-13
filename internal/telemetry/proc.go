package telemetry

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type cpuTimes struct {
	busy  uint64
	total uint64
}

// readCPU parses the aggregate "cpu" line of /proc/stat. Busy excludes idle and
// iowait so the figure matches ordinary "CPU utilisation" expectations.
func readCPU(path string) (cpuTimes, error) {
	f, err := os.Open(path)
	if err != nil {
		return cpuTimes{}, err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "cpu ") {
			continue
		}
		fields := strings.Fields(line)[1:]
		if len(fields) < 4 {
			return cpuTimes{}, fmt.Errorf("malformed cpu line in %s", path)
		}
		var t cpuTimes
		for i, field := range fields {
			v, err := strconv.ParseUint(field, 10, 64)
			if err != nil {
				// Kernel 3.10 has nine fields; later fields are optional.
				continue
			}
			t.total += v
			if i == 3 || i == 4 { // idle, iowait
				continue
			}
			t.busy += v
		}
		if t.total == 0 {
			return cpuTimes{}, fmt.Errorf("empty cpu counters in %s", path)
		}
		return t, nil
	}
	if err := sc.Err(); err != nil {
		return cpuTimes{}, err
	}
	return cpuTimes{}, fmt.Errorf("no cpu line in %s", path)
}

func cpuPercent(prev, cur cpuTimes) *float64 {
	if cur.total <= prev.total {
		return nil
	}
	dBusy := float64(cur.busy - prev.busy)
	dTotal := float64(cur.total - prev.total)
	if dTotal <= 0 {
		return nil
	}
	v := 100 * dBusy / dTotal
	if v < 0 {
		v = 0
	}
	if v > 100 {
		v = 100
	}
	return &v
}

type netCounters struct {
	rx, tx uint64
}

// readNetDev parses /proc/net/dev. This kernel uses 32-bit counters, so a
// decrease is treated as a single wrap rather than a negative rate.
func readNetDev(path string) (map[string]netCounters, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	out := make(map[string]netCounters)
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		name, rest, ok := strings.Cut(line, ":")
		if !ok {
			continue // header lines have no colon-delimited name
		}
		name = strings.TrimSpace(name)
		fields := strings.Fields(rest)
		if len(fields) < 9 {
			continue
		}
		rx, err1 := strconv.ParseUint(fields[0], 10, 64)
		tx, err2 := strconv.ParseUint(fields[8], 10, 64)
		if err1 != nil || err2 != nil {
			continue
		}
		out[name] = netCounters{rx: rx, tx: tx}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no interface counters in %s", path)
	}
	return out, nil
}

const counterWrap = 1 << 32

func delta32(newer, older uint64) uint64 {
	if newer >= older {
		return newer - older
	}
	return newer + counterWrap - older
}

// readMeminfo returns the fields this program needs, in kB.
func readMeminfo(path string) (map[string]int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	out := make(map[string]int64)
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 2 {
			continue
		}
		key := strings.TrimSuffix(fields[0], ":")
		v, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			continue
		}
		out[key] = v
	}
	return out, sc.Err()
}

// readLoad returns the three load averages.
func readLoad(path string) ([3]float64, error) {
	var out [3]float64
	b, err := os.ReadFile(path)
	if err != nil {
		return out, err
	}
	fields := strings.Fields(string(b))
	if len(fields) < 3 {
		return out, fmt.Errorf("malformed load average in %s", path)
	}
	for i := 0; i < 3; i++ {
		v, err := strconv.ParseFloat(fields[i], 64)
		if err != nil {
			return out, err
		}
		out[i] = v
	}
	return out, nil
}

// readUptime returns the first value of /proc/uptime in seconds.
func readUptime(path string) (float64, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	fields := strings.Fields(string(b))
	if len(fields) == 0 {
		return 0, fmt.Errorf("empty uptime in %s", path)
	}
	return strconv.ParseFloat(fields[0], 64)
}

func readFileInt(path string) (*int64, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	v, err := strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64)
	if err != nil {
		return nil, err
	}
	return &v, nil
}

func readFileString(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}
