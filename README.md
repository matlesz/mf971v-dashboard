# mf971v-dashboard

A small, **read-only** status dashboard for the ZTE MF971V LTE MiFi.

**Download:** [v0.1.0 release](https://github.com/matlesz/mf971v-dashboard/releases/tag/v0.1.0) —
a static ARMv7 binary plus `SHA256SUMS`.

One statically linked ARM binary with the UI compiled in. It polls the router's
own status API and reads `/proc` and `/sys`, then serves a responsive page. It
never writes to the router, never changes settings, and never proxies arbitrary
device commands.

```
┌─────────────────────────────┐        HTTP GET (allowlisted keys)
│ browser ──► dashboard :8080 │ ───────────────────────────────────► router
│                             │ ◄───────────────────────────────────  /goform
│            /proc /sys reads │
└─────────────────────────────┘
```

## Status

Early, honest state of the project:

- Unit tests cover parsing, counter wrap, thermal scaling, the API contract and
  the embedded UI.
- The API path was smoke-tested end to end against a real MF971V B02 from a
  workstation.
- The ARM binary **has been executed on a real MF971V B02**: it served the UI
  and live telemetry with no collector warnings, using about 5.5 MB RSS. Its
  unit handling (millivolt battery voltage, milliamp current, mixed thermal
  scales) was corrected from that run.
- The reference device has no usable SIM, so radio figures show as unavailable.
  That is the correct behaviour, not a bug.
- No autostart: running it on the router uses the vendor's USB debug mode, which
  disappears on restart. See [docs/compatibility.md](docs/compatibility.md).

## What it shows

| Group | Values |
| --- | --- |
| Connection | network type, PPP state, modem state, RSRP/RSCP/RSSI or signal bars |
| Power | battery percentage, charging state, health, voltage, signed current |
| System | CPU utilisation, load, memory, uptime, per-mount free space |
| Network | per-interface throughput and total counters |
| Thermal | every thermal zone, with per-zone unit handling |
| Usage | vendor monthly RX/TX and session time |
| Wi-Fi | radio on/off, channel, band, max clients, connected client count |

Values the vendor firmware does **not** expose (RSRQ, SINR, band, cell ID, PCI,
EARFCN, carrier aggregation) are not invented. See
[docs/compatibility.md](docs/compatibility.md) for the evidence behind that.

## Run on a workstation

The UI and API layer work anywhere Go runs; `/proc` and `/sys` readings simply
report warnings on a non-Linux host.

```sh
go run ./cmd/mf971v-dashboard -router http://192.168.0.1
# open http://127.0.0.1:8080
```

Flags:

| Flag | Default | Meaning |
| --- | --- | --- |
| `-listen` | `127.0.0.1:8080` | HTTP listen address |
| `-router` | `http://192.168.0.1` | vendor web origin to poll |
| `-interval` | `2s` | polling interval (minimum 500ms) |
| `-version` | | print version and exit |

## Build

```sh
make arm     # dist/mf971v-dashboard-linux-armv7, static, ARMv7
make test
```

Requires Go 1.24 or newer. No C toolchain, no cgo, no external Go modules.

## Install on the router (reversible, no autostart)

This uses the router's own USB debug mode, which is **not persistent**: a normal
restart returns USB to the factory ECM composition. Nothing is added to the boot
process, so the dashboard disappears on restart while the binary file remains.

1. On the router's web UI, while logged in, enable USB debug mode, then confirm
   the device enumerates with an ADB interface (see the research repository's
   mode-switch record for the exact, device-specific experiment).
2. Deploy to the writable cache partition and run it in the foreground:

   ```sh
   ./scripts/deploy.sh dist/mf971v-dashboard-linux-armv7
   ```

   This pushes to `/cache/mf971v-dashboard`, makes it executable, starts it on
   loopback and forwards port 8080 to your machine.
3. Open <http://127.0.0.1:8080>.
4. Restart the router to return to normal USB operation. The process stops with
   the restart.

Remove the binary (repeat the ADB setup first):

```sh
adb shell 'rm -f /cache/mf971v-dashboard'
```

The binary is about 7 MB, which matters on a device with roughly 60 MB free on
`/cache`. There is no autostart, no init script and no web-root modification,
because each of those is a persistent change with a separate risk profile.

## Security model

- Binds to `127.0.0.1` by default. Binding to a LAN address prints a warning and
  is your decision; there is no authentication and no TLS.
- Only reads. No settings writes, no shell execution, no arbitrary API keys:
  the key allowlist is compiled in and requests outside it are rejected.
- Requested keys deliberately exclude IMEI, IMSI, SMS content, USSD, SSIDs,
  WLAN keys and account credentials. Client lists are reduced to a count.
- The vendor API needs a browser-style `Referer` header; without it the router
  returns the requested keys with empty values. This program always sends it.

Report security issues as described in [SECURITY.md](SECURITY.md).

## Compatibility

Verified against `ZTE_MF971V_Fireware_B02` on ARMv7 with Linux 3.10. Details,
units and known pitfalls: [docs/compatibility.md](docs/compatibility.md).

## License

MIT. See [LICENSE](LICENSE). This project is unaffiliated with ZTE; the firmware,
its API and its data remain the vendor's.
