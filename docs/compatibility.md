# Compatibility and data notes

Everything here was observed read-only on one retail **ZTE MF971V** reporting
`ZTE_MF971V_Fireware_B02`. Other firmware revisions may differ. No device
identifiers are recorded.

## Platform

| Property | Value |
| --- | --- |
| CPU | ARMv7 rev3, VFP/NEON, hardware string Qualcomm MDM9630 |
| Kernel | Linux 3.10.0+ armv7l |
| RAM | ~140 MiB, no swap |
| C library | glibc from 2016 (`ld-linux.so.3`), so static linking is used |
| Init | sysvinit-style, scripts in `/etc/init.d` |
| Runtimes present | busybox, nc, openssl, sqlite3 |
| Runtimes absent | python, python3, lua, node, uhttpd, lighttpd, socat, jq |
| Free space | ~25–60 MiB depending on mount |

Consequences: the agent must be a compiled static binary; there is no scripting
runtime on the device, and the binary needs to stay small enough to fit the
cache partition comfortably.

## Vendor API

`GET /goform/goform_get_cmd_process?isTest=false&multi_data=1&cmd=k1,k2,...`

- Returns a JSON object of the requested keys.
- **A `Referer` header is required in practice.** Without it the service still
  answers HTTP 200 but every requested key is an empty string, which is
  indistinguishable from "unsupported" unless compared directly. This was
  reproduced from the device and from a workstation.
- Empty string means "no value right now", not zero. The collector maps it to
  `null` so the UI never shows a fake `0`.
- `station_list` is a JSON array encoded *inside* a string; identities in it are
  ignored, only the count is used.
- The key namespace also contains identifiers and secrets. This project uses a
  compiled-in allowlist and rejects anything else.

## Values that worked

Firmware/build strings, `wan_connect_status`, `network_type`, `ppp_status`,
`modem_main_state`, `dial_mode`, `rssi`, `rscp`, `RadioOff`, `Channel`,
`MAX_Access_num`, `wifi_coverage`, `wifi_5g_enable`, `battery_vol_percent`,
`battery_charging`, `battery_pers`, monthly RX/TX bytes and time, `date_month`,
`data_volume_limit_switch`, `lan_ipaddr`.

Kernel sources: 17/17 standard `power_supply` fields, 8/8 thermal zones, CPU
tick counters (100 Hz), memory, load, uptime, `/proc/net/dev`, statfs.

## Values that are empty and why

| Group | Reason |
| --- | --- |
| `realtime_*`, `wan_ipaddr`, `dhcp_wan_status`, `opms_wan_mode` | no active data session (`ppp_disconnected`) |
| `signalbar`, `lte_rsrp`, `pin_status`, `simcard_roam`, `network_provider` | no usable SIM / registration |
| `station_list`, `hostNameList` | no associated Wi-Fi clients during sampling |
| `wifi_enable`, some data-volume fields | returned empty; cause not established |

`network_type` is **not** a connectivity indicator. The reference unit reported
`LIMITED_SERVICE_WCDMA` together with SIM-undetected and disconnected states,
and its RSSI/RSCP values went empty after a restart, proving the earlier numbers
were stale. Gate every radio figure on `ppp_status`/`wan_connect_status`.

## Values not exposed at all

`lte_rsrq`, `lte_sinr`, `sinr`, `lte_snr`, `lte_band`, `wan_active_band`,
`lte_ca_status`, `cell_id`, `cellid`, `pci`, `earfcn`, `temperature`,
`battery_temperature`.

These names are absent from the shipped UI's 168-key inventory and return empty
when probed. Modem AT channels were not probed; obtaining band/cell/quality data
would be a separate investigation and is out of scope for this release.

## Units and pitfalls

- Thermal zones mix scales. SoC and PA sensors return plain degrees (37–42);
  the battery zone returns a value 100× larger (3600 vs the same temperature
  reported as 36 elsewhere). The collector scales by magnitude and marks
  `scale_assumed`, which the UI shows with an asterisk.
- `current_avg` is signed microamps; negative means discharging.
- `/proc/net/dev` counters are 32-bit here and can wrap; deltas handle a single
  wrap.
- The device clock is years behind, so all durations come from `/proc/uptime`
  and monotonic deltas. `clock_trusted` is reported and the UI notes it.
- CPU utilisation needs two ticks samples; the first sample after start is taken
  250 ms later to avoid opening with an empty gauge.

## Tested on hardware

Executed on the reference unit on 2026-11-13 (v0.1.0, static ARMv7 binary):

| Item | Result |
| --- | --- |
| Vendor API from a workstation over USB networking | works |
| Vendor API from the device itself | works |
| ARMv7 static build produced | yes, 7.2 MB |
| Binary executed on the router | yes — served the UI and live data |
| Memory footprint | ~5.5 MB RSS, ~560 MB virtual (Go address space reservation) |
| Collector warnings during the test | none |
| Autostart / persistence | deliberately not implemented |

What the live run corrected, compared with the first workstation-only build:

- `voltage_now` is **millivolts** (`4362` → 4.36 V) despite the kernel class
  documenting microvolts; `current_avg` is **milliamps** (`12` → +12 mA).
- The `battery` thermal zone (`3400`) needed the 100× scaling rule; the
  neighbouring `power_supply` temperature (`34`) is already Celsius.
- A UI bug in the sparkline series initialisation was only visible in a browser,
  not in unit tests; it is fixed.
- Radio figures were present (`RSCP -86`) while `ppp_status` was
  `ppp_disconnected` and the SIM was undetected, confirming they are stale
  leftovers. The UI now withholds them unless a data session is up.

Because the test ran in the vendor's USB debug mode, the device reported the
Wi-Fi radio as off and `0 bit/s` on every interface; the counters themselves
were read correctly. Wi-Fi client counts and real throughput therefore remain
unverified on this unit.

If you run it on a different firmware, open an issue with the firmware string,
the `curl` output of the unauthenticated status endpoint (redact identifiers)
and any metric that renders incorrectly.
