package telemetry

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// allowlist is the complete set of vendor API keys this program may request.
// The shipped key namespace also contains subscriber identifiers, SSIDs and
// credentials; nothing outside this list is ever asked for, and callers cannot
// extend it at runtime.
var allowlist = []string{
	"wa_inner_version", "hardware_version",
	"network_type", "ppp_status", "modem_main_state",
	"rssi", "rscp", "lte_rsrp", "signalbar", "wan_connect_status",
	"battery_vol_percent", "battery_charging", "battery_pers",
	"monthly_rx_bytes", "monthly_tx_bytes", "monthly_time",
	"realtime_rx_bytes", "realtime_tx_bytes", "realtime_time",
	"realtime_rx_thrpt", "realtime_tx_thrpt",
	"RadioOff", "Channel", "MAX_Access_num", "wifi_band", "wifi_5g_enable",
	"station_list",
}

// API talks to the on-device ZTE backend. The vendor endpoint requires a
// browser-style Referer: without it the service answers HTTP 200 with the
// requested keys present but empty, which is indistinguishable from "no data".
type API struct {
	BaseURL string
	Client  *http.Client
}

// NewAPI returns a client for the given origin, e.g. http://192.168.0.1.
func NewAPI(baseURL string) *API {
	return &API{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Client:  &http.Client{Timeout: 5 * time.Second},
	}
}

// Fetch performs one batched read-only request. Keys not on the allowlist are
// rejected rather than silently dropped.
func (a *API) Fetch(ctx context.Context, keys []string) (map[string]string, error) {
	allowed := make(map[string]bool, len(allowlist))
	for _, k := range allowlist {
		allowed[k] = true
	}
	for _, k := range keys {
		if !allowed[k] {
			return nil, fmt.Errorf("key %q is not on the read-only allowlist", k)
		}
	}

	q := url.Values{}
	q.Set("isTest", "false")
	q.Set("multi_data", "1")
	q.Set("cmd", strings.Join(keys, ","))
	endpoint := a.BaseURL + "/goform/goform_get_cmd_process?" + q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Referer", a.BaseURL+"/index.html")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")

	resp, err := a.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("vendor API returned HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 128<<10))
	if err != nil {
		return nil, err
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("vendor API returned non-JSON body: %w", err)
	}

	out := make(map[string]string, len(raw))
	for k, v := range raw {
		// Values are normally JSON strings, but tolerate a bare number/object.
		var s string
		if err := json.Unmarshal(v, &s); err == nil {
			out[k] = s
		} else {
			out[k] = strings.TrimSpace(string(v))
		}
	}
	return out, nil
}

func str(m map[string]string, key string) string {
	return strings.TrimSpace(m[key])
}

func intPtr(m map[string]string, key string) *int {
	s := str(m, key)
	if s == "" {
		return nil
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return nil
	}
	return &v
}

func int64Ptr(m map[string]string, key string) *int64 {
	s := str(m, key)
	if s == "" {
		return nil
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return nil
	}
	return &v
}

func floatPtr(m map[string]string, key string) *float64 {
	s := str(m, key)
	if s == "" {
		return nil
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil
	}
	return &v
}

func bool01Ptr(m map[string]string, key string) *bool {
	s := str(m, key)
	if s == "" {
		return nil
	}
	v := s == "1" || strings.EqualFold(s, "true")
	return &v
}

// clientCount parses station_list, which is a JSON array encoded as a string
// inside the response. Identities in that array are deliberately ignored.
func clientCount(m map[string]string) *int {
	s := str(m, "station_list")
	if s == "" {
		return nil
	}
	var list []json.RawMessage
	if err := json.Unmarshal([]byte(s), &list); err != nil {
		return nil
	}
	n := len(list)
	return &n
}
