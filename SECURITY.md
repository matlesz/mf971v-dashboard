# Security policy

## Intended deployment

`mf971v-dashboard` is a read-only telemetry viewer for a device on a trusted
network. It is not hardened for exposure to the internet and must not be
port-forwarded, reverse-proxied without authentication, or published as a
public service.

## Design guarantees

- The binary performs only file reads under `/proc` and `/sys`, plus HTTP GETs
  to a single configurable origin.
- The vendor API key list is compiled in. There is no parameter, endpoint or
  flag that lets a client request other keys, run a command, or write a setting.
- IMEI, IMSI, SSIDs, WLAN keys, SMS/USSD bodies and account credentials are not
  requested. Wi-Fi clients are reduced to a count inside the collector.
- The listener binds to loopback by default. Binding elsewhere is explicit and
  logged as a warning.
- Responses carry a restrictive Content-Security-Policy, `nosniff` and
  `no-referrer`; the UI inserts data with `textContent`, not HTML.

## Known limitations

- No authentication and no TLS: anyone who can reach the listening port can read
  the telemetry. Keep it on loopback or a trusted, access-controlled network.
- The router's own admin UI is served over plain HTTP; this project cannot fix
  that.
- Telemetry accuracy depends on vendor values that are partly undocumented.
  Unit guesses are flagged in the UI rather than silently trusted.
- Running the binary on the router requires enabling the vendor's USB debug
  mode, which grants root-level ADB access while active. Treat the USB cable and
  the host machine as part of the trusted environment.

## Reporting a vulnerability

Open a private security advisory on the repository, or contact the maintainer
through the repository profile. Please include the version, the command line
used, and what an attacker could achieve. Do not include router serial numbers,
subscriber identifiers, credentials or packet captures with personal data.

There is no bug bounty and no response-time guarantee.
