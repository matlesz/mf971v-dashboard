// Read-only dashboard. Renders whatever /api/snapshot returns; never mutates
// the device and never inserts remote strings as HTML.
"use strict";

const $ = (id) => document.getElementById(id);

const setText = (id, value) => {
  const node = $(id);
  if (node && node.textContent !== value) node.textContent = value;
};

const fmtBytes = (bytes) => {
  if (bytes === null || bytes === undefined || !isFinite(bytes)) return "–";
  const units = ["B", "KiB", "MiB", "GiB", "TiB"];
  let v = Math.abs(bytes);
  let i = 0;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i += 1;
  }
  const sign = bytes < 0 ? "-" : "";
  return `${sign}${v.toFixed(v >= 100 || i === 0 ? 0 : 1)} ${units[i]}`;
};

const fmtRate = (bps) => {
  if (bps === null || bps === undefined || !isFinite(bps)) return "–";
  if (bps < 1000) return `${bps.toFixed(0)} bit/s`;
  if (bps < 1e6) return `${(bps / 1e3).toFixed(1)} kbit/s`;
  if (bps < 1e9) return `${(bps / 1e6).toFixed(2)} Mbit/s`;
  return `${(bps / 1e9).toFixed(2)} Gbit/s`;
};

const fmtDuration = (seconds) => {
  if (!seconds || !isFinite(seconds) || seconds < 0) return "–";
  const s = Math.floor(seconds);
  const d = Math.floor(s / 86400);
  const h = Math.floor((s % 86400) / 3600);
  const m = Math.floor((s % 3600) / 60);
  if (d) return `${d}d ${h}h ${m}m`;
  if (h) return `${h}h ${m}m ${s % 60}s`;
  if (m) return `${m}m ${s % 60}s`;
  return `${s}s`;
};

const fmtTemp = (c, assumed) => {
  if (c === null || c === undefined || !isFinite(c)) return "–";
  return `${c.toFixed(1)} °C${assumed ? " *" : ""}`;
};

const fmtSigned = (v, unit, digits = 0) => {
  if (v === null || v === undefined || !isFinite(v)) return "–";
  return `${v > 0 ? "+" : ""}${v.toFixed(digits)} ${unit}`;
};

const humanize = (value) => {
  if (!value) return "–";
  return String(value).replace(/_/g, " ");
};

// Sparkline helper with a fixed-size ring buffer.
class Spark {
  constructor(canvas, seriesCount, colors) {
    this.canvas = canvas;
    this.count = seriesCount;
    this.colors = colors;
    this.capacity = 60;
    this.data = Array.from({ length: seriesCount }, () => []);
    this.resize();
    window.addEventListener("resize", () => {
      this.resize();
      this.draw();
    });
  }

  resize() {
    const dpr = window.devicePixelRatio || 1;
    const rect = this.canvas.getBoundingClientRect();
    this.canvas.width = Math.max(1, Math.round(rect.width * dpr));
    this.canvas.height = Math.max(1, Math.round(rect.height * dpr));
    this.ctx = this.canvas.getContext("2d");
    this.ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    this.width = rect.width;
    this.height = rect.height;
  }

  push(values) {
    values.forEach((value, index) => {
      const series = this.data[index];
      series.push(Number.isFinite(value) ? value : null);
      if (series.length > this.capacity) series.shift();
    });
    this.draw();
  }

  draw() {
    const { ctx, width, height } = this;
    if (!ctx || !width || !height) return;
    ctx.clearRect(0, 0, width, height);
    const all = this.data.flat().filter((v) => v !== null);
    const max = Math.max(1, ...all);
    ctx.lineWidth = 1.5;
    this.data.forEach((series, index) => {
      if (series.length < 2) return;
      ctx.beginPath();
      let started = false;
      series.forEach((value, i) => {
        if (value === null) {
          started = false;
          return;
        }
        const x = (i / (this.capacity - 1)) * width;
        const y = height - (value / max) * (height - 4) - 2;
        if (!started) {
          ctx.moveTo(x, y);
          started = true;
        } else {
          ctx.lineTo(x, y);
        }
      });
      ctx.strokeStyle = this.colors[index];
      ctx.stroke();
    });
  }
}

let cpuSpark;
let netSpark;
let lastFetch = 0;
let lastOk = false;

function signalText(cell) {
  // Radio figures can be stale leftovers from an earlier session: this firmware
  // reported non-empty RSCP while the modem said SIM undetected. Only trust them
  // when the data session itself is up.
  if (!cell.service) return "unavailable (no data session)";
  if (cell.rsrp_dbm !== null && cell.rsrp_dbm !== undefined) return `${cell.rsrp_dbm} dBm (RSRP)`;
  if (cell.rscp_dbm !== null && cell.rscp_dbm !== undefined) return `${cell.rscp_dbm} dBm (RSCP)`;
  if (cell.rssi_dbm !== null && cell.rssi_dbm !== undefined) return `${cell.rssi_dbm} dBm (RSSI)`;
  if (cell.signal_bar !== null && cell.signal_bar !== undefined) return `bar ${cell.signal_bar}`;
  return "–";
}

function render(snap) {
  const device = snap.device || {};
  const system = snap.system || {};
  const power = snap.power || {};
  const cell = snap.cell || {};
  const usage = snap.usage || {};
  const wifi = snap.wifi || {};

  const fw = device.firmware || "unknown firmware";
  const hw = device.hardware ? ` · ${device.hardware}` : "";
  setText("firmware", `${fw}${hw}`);

  const pill = $("service-pill");
  if (cell.service) {
    pill.textContent = "Online";
    pill.className = "pill pill-good";
  } else if ((cell.modem_state || "").includes("sim_undetected")) {
    pill.textContent = "No SIM";
    pill.className = "pill pill-warn";
  } else {
    pill.textContent = "No data";
    pill.className = "pill pill-warn";
  }
  pill.title = cell.network_type ? `Reported technology: ${cell.network_type}` : "";

  setText("network-type", humanize(cell.network_type));
  setText("ppp-status", humanize(cell.ppp_status));
  setText("modem-state", humanize(cell.modem_state));
  setText("signal", signalText(cell));

  setText("battery-pct", power.battery_percent === null || power.battery_percent === undefined ? "–" : `${power.battery_percent}%`);
  const ring = document.querySelector(".battery-ring");
  if (ring) ring.style.setProperty("--pct", String(power.battery_percent ?? 0));
  setText("battery-status", power.status || "–");
  setText("battery-health", power.health || "–");
  setText("battery-voltage", power.voltage_v === null || power.voltage_v === undefined ? "–" : `${power.voltage_v.toFixed(2)} V`);
  setText("battery-current", fmtSigned(power.current_ma, "mA"));
  setText("battery-temp", fmtTemp(power.temp_c, false));

  setText("cpu", system.cpu_percent === null || system.cpu_percent === undefined ? "–" : `${system.cpu_percent.toFixed(0)}%`);
  setText("load", system.load1 === undefined ? "" : `load ${Number(system.load1).toFixed(2)}`);
  if (cpuSpark) cpuSpark.push([system.cpu_percent ?? null]);

  const memUsed = system.mem_used_percent;
  setText("mem-used", memUsed === null || memUsed === undefined ? "–" : `${memUsed.toFixed(0)}%`);
  const memBar = $("mem-bar");
  if (memBar) memBar.style.width = `${Math.max(0, Math.min(100, memUsed ?? 0))}%`;
  setText(
    "mem-detail",
    system.mem_total_kb
      ? `${fmtBytes((system.mem_total_kb - system.mem_available_kb) * 1024)} of ${fmtBytes(system.mem_total_kb * 1024)} used`
      : "–"
  );

  const interfaces = Array.isArray(system.interfaces) ? system.interfaces : [];
  const usable = interfaces.filter((i) => !i.name.startsWith("lo"));
  const totals = usable.reduce((acc, i) => ({ rx: acc.rx + i.rx_bps, tx: acc.tx + i.tx_bps }), { rx: 0, tx: 0 });
  if (netSpark) netSpark.push([totals.rx, totals.tx]);
  setText("net-summary", `RX ${fmtRate(totals.rx)} · TX ${fmtRate(totals.tx)}`);

  setText("usage-month", `${fmtBytes(usage.month_rx_bytes)} ↓ · ${fmtBytes(usage.month_tx_bytes)} ↑`);
  setText("usage-time", fmtDuration(usage.month_time_seconds));
  setText("uptime", fmtDuration(device.uptime_seconds));

  setText("wifi-radio", wifi.radio_off === null || wifi.radio_off === undefined ? "–" : wifi.radio_off ? "Off" : "On");
  setText("wifi-channel", wifi.channel === null || wifi.channel === undefined ? "–" : String(wifi.channel));
  setText("wifi-band", [wifi.band ? `${wifi.band}-band` : null, wifi.five_ghz ? "5 GHz enabled" : null].filter(Boolean).join(" · ") || "–");
  setText("wifi-clients", wifi.clients === null || wifi.clients === undefined ? "–" : String(wifi.clients));

  renderTemps(snap.thermal || []);
  renderDisks(system.disks || []);
  renderInterfaces(usable);
  renderWarnings(snap.warnings || [], device.clock_trusted);
}

function renderTemps(zones) {
  const list = $("temps");
  if (!list) return;
  list.replaceChildren();
  if (!zones.length) {
    list.append(emptyItem("No thermal zones reported"));
    return;
  }
  zones.forEach((zone) => {
    const li = document.createElement("li");
    const name = document.createElement("span");
    name.className = "name";
    name.textContent = `${zone.index} · ${zone.type}`;
    const value = document.createElement("span");
    value.textContent = fmtTemp(zone.celsius, zone.scale_assumed);
    if (zone.scale_assumed) value.title = `raw value ${zone.raw}; scale inferred`;
    li.append(name, value);
    list.append(li);
  });
}

function renderDisks(disks) {
  const list = $("disks");
  if (!list) return;
  list.replaceChildren();
  if (!disks.length) {
    list.append(emptyItem("No mounted filesystems reported"));
    return;
  }
  disks.forEach((disk) => {
    const li = document.createElement("li");
    const label = document.createElement("span");
    label.className = "label";
    label.textContent = disk.mount;
    const meter = document.createElement("span");
    meter.className = "meter";
    const fill = document.createElement("span");
    fill.style.width = `${Math.max(0, Math.min(100, disk.used_percent ?? 0))}%`;
    meter.append(fill);
    const value = document.createElement("span");
    value.className = "value";
    value.textContent = `${fmtBytes(disk.free_kb * 1024)} free`;
    li.append(label, meter, value);
    list.append(li);
  });
}

function renderInterfaces(interfaces) {
  const body = $("ifaces");
  if (!body) return;
  body.replaceChildren();
  if (!interfaces.length) {
    const tr = document.createElement("tr");
    const td = document.createElement("td");
    td.colSpan = 5;
    td.className = "muted";
    td.textContent = "No interface counters yet";
    tr.append(td);
    body.append(tr);
    return;
  }
  interfaces
    .slice()
    .sort((a, b) => b.rx_bps + b.tx_bps - (a.rx_bps + a.tx_bps))
    .forEach((iface) => {
      const tr = document.createElement("tr");
      [
        iface.name,
        fmtRate(iface.rx_bps),
        fmtRate(iface.tx_bps),
        fmtBytes(iface.rx_bytes),
        fmtBytes(iface.tx_bytes),
      ].forEach((text) => {
        const td = document.createElement("td");
        td.textContent = text;
        tr.append(td);
      });
      body.append(tr);
    });
}

function renderWarnings(warnings, clockTrusted) {
  const notes = warnings.slice();
  if (clockTrusted === false) notes.push("Device clock is far behind; timestamps are not used for durations.");
  const wrap = $("warnings-wrap");
  const list = $("warnings");
  if (!wrap || !list) return;
  if (!notes.length) {
    wrap.hidden = true;
    list.replaceChildren();
    return;
  }
  wrap.hidden = false;
  setText("warning-count", `(${notes.length})`);
  list.replaceChildren();
  notes.forEach((note) => {
    const li = document.createElement("li");
    li.textContent = note;
    list.append(li);
  });
}

function emptyItem(text) {
  const li = document.createElement("li");
  li.className = "muted";
  li.textContent = text;
  return li;
}

async function tick() {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), 5000);
  try {
    const response = await fetch("/api/snapshot", { signal: controller.signal, cache: "no-store" });
    if (!response.ok) throw new Error(`HTTP ${response.status}`);
    render(await response.json());
    lastOk = true;
    lastFetch = Date.now();
  } catch (error) {
    lastOk = false;
    const pill = $("service-pill");
    if (pill) {
      pill.textContent = "Agent offline";
      pill.className = "pill pill-bad";
      pill.title = String(error);
    }
  } finally {
    clearTimeout(timer);
  }
  const updated = $("updated");
  if (updated) {
    if (!lastOk) {
      updated.textContent = "no data";
    } else {
      const age = Math.round((Date.now() - lastFetch) / 1000);
      updated.textContent = age <= 1 ? "updated just now" : `updated ${age}s ago`;
    }
  }
}

function boot() {
  cpuSpark = new Spark($("cpu-spark"), 1, ["#4cc2ff"]);
  netSpark = new Spark($("net-spark"), 2, ["#4cc2ff", "#3ddc97"]);
  tick();
  setInterval(tick, 2000);
}

boot();
