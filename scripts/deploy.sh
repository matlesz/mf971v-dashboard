#!/bin/sh
# Deploy the dashboard to the router over ADB and start it in the foreground.
#
# Requirements:
#   - the router is in USB debug mode and visible to `adb devices`
#   - the binary was built for linux/arm (make arm)
#
# This writes exactly one file (/cache/mf971v-dashboard) and starts one process.
# It does not add autostart, does not touch /etc, /usr or the vendor web root,
# and a router restart removes the running process.
set -eu

BIN=${1:-dist/mf971v-dashboard-linux-armv7}
REMOTE=/cache/mf971v-dashboard
PORT=${PORT:-8080}

if [ ! -f "$BIN" ]; then
  echo "binary not found: $BIN (run: make arm)" >&2
  exit 1
fi

if ! adb get-state >/dev/null 2>&1; then
  echo "no ADB device. Enable USB debug mode on the router first." >&2
  exit 1
fi

echo "==> pushing $(basename "$BIN") to $REMOTE"
adb push "$BIN" "$REMOTE" >/dev/null
adb shell "chmod 755 $REMOTE"

echo "==> stopping any previous instance"
adb shell "kill \$(pidof mf971v-dashboard) 2>/dev/null || true"

echo "==> starting on 127.0.0.1:$PORT"
# The trailing `sleep 1` matters: adb shell tears down the session as soon as the
# command returns, which kills a freshly backgrounded child before it can even
# open its log file.
adb shell "cd /cache && nohup $REMOTE -listen 127.0.0.1:$PORT -router http://192.168.0.1 >/cache/mf971v-dashboard.log 2>&1 & sleep 1"

if ! adb shell 'pidof mf971v-dashboard' >/dev/null 2>&1; then
  echo "start failed; last log lines:" >&2
  adb shell 'tail -n 20 /cache/mf971v-dashboard.log' >&2 || true
  exit 1
fi

sleep 1
adb forward "tcp:$PORT" "tcp:$PORT" >/dev/null
echo "==> open http://127.0.0.1:$PORT"
echo "    logs: adb shell cat /cache/mf971v-dashboard.log"
echo
echo "To stop:    adb shell 'kill \$(pidof mf971v-dashboard)'"
echo "To remove:  adb shell 'rm -f $REMOTE /cache/mf971v-dashboard.log'"
echo "To restore: restart the router (returns USB to normal mode)"
