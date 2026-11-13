# Changelog

## v0.1.0 — first release

Published as [v0.1.0](https://github.com/matlesz/mf971v-dashboard/releases/tag/v0.1.0)
(`sha256 161c0711…a1bb04`, 6,422,712 bytes, static ARMv7).

- Read-only collector for the MF971V status API with a compiled-in key
  allowlist and the required `Referer` header.
- `/proc` and `/sys` collectors: CPU, load, memory, uptime, per-mount free
  space, interface rates with 32-bit counter wrap handling, `power_supply`
  fields and thermal zones with mixed-scale detection.
- Embedded single-page UI: responsive grid, dark/light via `prefers-color-scheme`,
  CPU and throughput sparklines, explicit "no SIM / no data" states.
- Endpoints: `/`, `/api/snapshot`, `/healthz`. Loopback binding by default,
  restrictive CSP, no inline scripts.
- Static `linux/arm` (ARMv7) build target, `make arm`, and a tag-driven release
  workflow producing a checksummed binary.
- `scripts/deploy.sh` for a one-file, non-persistent install over ADB.
- Unit tests for parsing, wrap handling, thermal scaling, API contract and the
  embedded UI.
- Verified on a real MF971V B02 (Linux 3.10, ARMv7): UI served, no collector
  warnings, ~5.5 MB RSS. Fixes from that run: battery voltage is millivolts,
  current is milliamps, battery temperature is Celsius, and the battery thermal
  zone needs the 100× rule. A sparkline initialisation bug and a stale-signal
  display bug were found in the browser and fixed.

Not done: autostart, historical storage, authentication, TLS, and any write or
settings capability.
