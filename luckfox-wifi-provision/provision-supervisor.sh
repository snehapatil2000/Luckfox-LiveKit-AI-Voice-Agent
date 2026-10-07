#!/bin/sh
set -u

INSTALL_DIR=/opt/luckfox-wifi-provision
LOG_FILE=/var/log/luckfox-wifi-provision.log
AGENT_PID=/var/run/luckfox-bt-agent.pid

log() {
    printf '%s %s\n' "$(date -Iseconds 2>/dev/null || date)" "$*" >> "$LOG_FILE"
}

cleanup() {
    rfcomm release /dev/rfcomm0 >/dev/null 2>&1 || true
    if [ -f "$AGENT_PID" ]; then
        kill "$(cat "$AGENT_PID")" >/dev/null 2>&1 || true
        rm -f "$AGENT_PID"
    fi
}

trap cleanup EXIT INT TERM

[ -S /var/run/dbus/system_bus_socket ] || /etc/init.d/S30dbus start >> "$LOG_FILE" 2>&1
pidof bluetoothd >/dev/null 2>&1 || /etc/init.d/S40bluetoothd start >> "$LOG_FILE" 2>&1
sleep 2

hciconfig hci0 up >> "$LOG_FILE" 2>&1 || exit 1
hciconfig hci0 name Luckfox-WiFi-Setup >> "$LOG_FILE" 2>&1
hciconfig hci0 piscan >> "$LOG_FILE" 2>&1 || exit 1

# Use Just Works for the app's direct RFCOMM channel.
bt-agent -c NoInputNoOutput >> "$LOG_FILE" 2>&1 &
echo $! > "$AGENT_PID"

sdptool add --channel=4 SP >> "$LOG_FILE" 2>&1
log "Bluetooth Wi-Fi provisioning ready as Luckfox-WiFi-Setup"

# Stop advertising after one hour, but remain connectable for paired phones.
( sleep 3600; hciconfig hci0 pscan >> "$LOG_FILE" 2>&1 ) &

while :; do
    rfcomm release /dev/rfcomm0 >/dev/null 2>&1 || true
    rm -f /dev/rfcomm0 >/dev/null 2>&1 || true
    log "RFCOMM listener starting channel=4 mode=listen"
    # Keep exactly one listener in the foreground. This Buildroot rfcomm
    # reliably creates the TTY and runs the handler with this form.
    rfcomm -r listen /dev/rfcomm0 4 "$INSTALL_DIR/handle-connection.sh" >> "$LOG_FILE" 2>&1 || true
    log "RFCOMM listener exited; retrying"
    sleep 1
done
