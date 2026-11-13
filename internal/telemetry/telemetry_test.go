package telemetry

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestScaleTemp(t *testing.T) {
	cases := []struct {
		raw     int64
		want    float64
		assumed bool
	}{
		{42, 42, false},
		{36, 36, false},
		{-5, -5, false},
		{420, 42, true},
		{3600, 36, true},
		{42000, 42, true},
	}
	for _, tc := range cases {
		got, assumed := scaleTemp(tc.raw)
		if got != tc.want || assumed != tc.assumed {
			t.Errorf("scaleTemp(%d) = (%v,%v), want (%v,%v)", tc.raw, got, assumed, tc.want, tc.assumed)
		}
	}
}

func TestPowerUnitScaling(t *testing.T) {
	// This driver reports millivolts and milliamps even though the kernel class
	// documents micro units; both encodings must be accepted.
	if v, ok := scaleVoltage(4362); !ok || v < 4.3 || v > 4.4 {
		t.Fatalf("voltage 4362 = %v,%v", v, ok)
	}
	if v, ok := scaleVoltage(4_362_000); !ok || v < 4.3 || v > 4.4 {
		t.Fatalf("voltage microvolts = %v,%v", v, ok)
	}
	if _, ok := scaleVoltage(12); ok {
		t.Fatal("implausible voltage must be rejected, not shown")
	}
	if ma, ok := scaleCurrent(12); !ok || ma != 12 {
		t.Fatalf("current 12 = %v,%v", ma, ok)
	}
	if ma, ok := scaleCurrent(-66); !ok || ma != -66 {
		t.Fatalf("negative current must stay negative, got %v", ma)
	}
	if c, ok := scaleBatteryTemp(34); !ok || c != 34 {
		t.Fatalf("battery temp = %v,%v", c, ok)
	}
}

func TestCPUPercent(t *testing.T) {
	prev := cpuTimes{busy: 3968, total: 350300}
	cur := cpuTimes{busy: 4068, total: 350400}
	got := cpuPercent(prev, cur)
	if got == nil {
		t.Fatal("expected a value")
	}
	if *got < 99 || *got > 100 {
		t.Fatalf("got %.2f, want 100 for a fully busy interval", *got)
	}

	idle := cpuPercent(cpuTimes{busy: 0, total: 100}, cpuTimes{busy: 0, total: 200})
	if idle == nil || *idle != 0 {
		t.Fatalf("got %v, want 0", idle)
	}
	if cpuPercent(cpuTimes{total: 100}, cpuTimes{total: 100}) != nil {
		t.Fatal("a zero-length interval must yield nil, not 0")
	}
}

func TestDelta32Wraps(t *testing.T) {
	if got := delta32(500, 400); got != 100 {
		t.Fatalf("normal delta = %d, want 100", got)
	}
	if got := delta32(10, counterWrap-10); got != 20 {
		t.Fatalf("wrapped delta = %d, want 20", got)
	}
}

func TestReadCPU(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stat")
	line := "cpu  3968 0 13584 331444 1320 0 3526 0 0 0\ncpu0 1 2 3 4 5 6 7 8 9 10\n"
	if err := os.WriteFile(path, []byte(line), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := readCPU(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.total != 3968+13584+331444+1320+3526 {
		t.Fatalf("total = %d", got.total)
	}
	if got.busy != 3968+13584+3526 {
		t.Fatalf("busy = %d", got.busy)
	}
}

func TestReadNetDev(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dev")
	body := "Inter-|   Receive                                                |  Transmit\n" +
		" face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop\n" +
		" wlan0: 9213669 51488 0 0 0 0 0 0 3943563 13408 0 12 0 0 0 0\n" +
		"  sit0:       0       0    0    0    0     0          0         0        0       0    0    0    0     0       0          0\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := readNetDev(path)
	if err != nil {
		t.Fatal(err)
	}
	if got["wlan0"].rx != 9213669 || got["wlan0"].tx != 3943563 {
		t.Fatalf("wlan0 = %+v", got["wlan0"])
	}
	if _, ok := got["sit0"]; !ok {
		t.Fatal("expected sit0 to be parsed")
	}
}

func TestThermalMixedScales(t *testing.T) {
	root := t.TempDir()
	write := func(zone, name, value string) {
		dir := filepath.Join(root, "class", "thermal", zone)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(value), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("thermal_zone0", "type", "tsens_tz_sensor0")
	write("thermal_zone0", "temp", "42")
	write("thermal_zone7", "type", "battery")
	write("thermal_zone7", "temp", "3600")

	zones, warnings := collectThermal(root)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(zones) != 2 {
		t.Fatalf("got %d zones", len(zones))
	}
	if zones[0].C == nil || *zones[0].C != 42 || zones[0].ScaleAssumed {
		t.Fatalf("zone0 = %+v", zones[0])
	}
	if zones[1].C == nil || *zones[1].C != 36 || !zones[1].ScaleAssumed {
		t.Fatalf("zone7 = %+v", zones[1])
	}
}

func TestClientCountIgnoresIdentities(t *testing.T) {
	values := map[string]string{
		"station_list": `[{"mac":"aa","hostname":"x"},{"mac":"bb"}]`,
	}
	got := clientCount(values)
	if got == nil || *got != 2 {
		t.Fatalf("got %v, want 2", got)
	}
	if clientCount(map[string]string{"station_list": ""}) != nil {
		t.Fatal("empty list must be nil")
	}
	if clientCount(map[string]string{"station_list": "not json"}) != nil {
		t.Fatal("garbage must be nil")
	}
}

func TestAPISendsRefererAndParses(t *testing.T) {
	var gotReferer, gotCmd string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotReferer = r.Header.Get("Referer")
		gotCmd = r.URL.Query().Get("cmd")
		fmt.Fprint(w, `{"network_type":"LIMITED_SERVICE_WCDMA","ppp_status":"ppp_disconnected","rssi":"-80","lte_rsrp":"","station_list":"[]"}`)
	}))
	defer srv.Close()

	api := NewAPI(srv.URL)
	values, err := api.Fetch(context.Background(), []string{"network_type", "rssi"})
	if err != nil {
		t.Fatal(err)
	}
	if gotReferer != srv.URL+"/index.html" {
		t.Fatalf("Referer = %q", gotReferer)
	}
	if gotCmd != "network_type,rssi" {
		t.Fatalf("cmd = %q", gotCmd)
	}
	if values["network_type"] != "LIMITED_SERVICE_WCDMA" {
		t.Fatalf("values = %v", values)
	}
	if intPtr(values, "missing") != nil {
		t.Fatal("missing keys must be nil")
	}
	if p := intPtr(values, "rssi"); p == nil || *p != -80 {
		t.Fatalf("rssi = %v", p)
	}
	if p := intPtr(values, "lte_rsrp"); p != nil {
		t.Fatal("empty string must be nil, not 0")
	}
}

func TestAPIRejectsUnknownKey(t *testing.T) {
	api := NewAPI("http://127.0.0.1:1")
	if _, err := api.Fetch(context.Background(), []string{"imei"}); err == nil {
		t.Fatal("expected rejection of a key outside the allowlist")
	}
}

func TestSamplerReadsFakeTree(t *testing.T) {
	proc := t.TempDir()
	must := func(path, body string) {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	must(filepath.Join(proc, "uptime"), "3650.41 3385.76\n")
	must(filepath.Join(proc, "stat"), "cpu  3968 0 13584 331444 1320 0 3526 0 0 0\n")
	must(filepath.Join(proc, "loadavg"), "4.10 4.17 4.21 1/314 15015\n")
	must(filepath.Join(proc, "meminfo"), "MemTotal: 143448 kB\nMemFree: 18800 kB\nBuffers: 0 kB\nCached: 59140 kB\n")
	must(filepath.Join(proc, "net", "dev"), "Inter-|\n face |\n wlan0: 100 1 0 0 0 0 0 0 50 1 0 0 0 0 0 0\n")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"wa_inner_version":"ZTE_MF971V_Fireware_B02","network_type":"LIMITED_SERVICE_WCDMA","ppp_status":"ppp_disconnected","modem_main_state":"modem_sim_undetected","battery_vol_percent":"86","monthly_rx_bytes":"16912620","RadioOff":"0","Channel":"0","MAX_Access_num":"32","station_list":""}`)
	}))
	defer srv.Close()

	s := NewSampler(NewAPI(srv.URL))
	s.ProcRoot = proc
	s.SysRoot = t.TempDir() // no sysfs here: warnings are expected, not fatal
	s.Mounts = nil
	s.Prime = 10 * time.Millisecond

	snap := s.Snapshot(context.Background())
	if snap.Device.Firmware != "ZTE_MF971V_Fireware_B02" {
		t.Fatalf("firmware = %q", snap.Device.Firmware)
	}
	if snap.Device.UptimeSeconds < 3650 || snap.Device.UptimeSeconds > 3651 {
		t.Fatalf("uptime = %v", snap.Device.UptimeSeconds)
	}
	// A static fake tree has no tick delta, so the first sample reports nil CPU
	// rather than a fabricated 0%.
	if snap.System.CPUPercent != nil {
		t.Fatalf("static counters must not produce a CPU figure, got %.2f", *snap.System.CPUPercent)
	}
	// Advance the counters the way a live kernel would, then sample again.
	must(filepath.Join(proc, "stat"), "cpu  3968 0 13585 331543 1321 0 3526 0 0 0\n")
	must(filepath.Join(proc, "net", "dev"), "Inter-|\n face |\n wlan0: 200 2 0 0 0 0 0 0 100 2 0 0 0 0 0 0\n")
	time.Sleep(120 * time.Millisecond)
	snap = s.Snapshot(context.Background())
	if snap.System.CPUPercent == nil {
		t.Fatal("CPU should be available once the counters advance")
	}
	if len(snap.System.Interfaces) != 1 {
		t.Fatalf("interfaces = %+v", snap.System.Interfaces)
	}
	if snap.System.Interfaces[0].RxBps <= 0 {
		t.Fatalf("rx rate = %v", snap.System.Interfaces[0].RxBps)
	}
	if snap.System.MemTotalKB != 143448 || snap.System.MemAvailableKB != 18800+59140 {
		t.Fatalf("memory = total %d available %d", snap.System.MemTotalKB, snap.System.MemAvailableKB)
	}
	if snap.Power.BatteryPercent == nil || *snap.Power.BatteryPercent != 86 {
		t.Fatalf("battery = %v", snap.Power.BatteryPercent)
	}
	if snap.Cell.Service {
		t.Fatal("ppp_disconnected must not count as service")
	}
	if snap.WiFi.Clients != nil {
		t.Fatalf("empty station_list must be nil, got %v", snap.WiFi.Clients)
	}
}
