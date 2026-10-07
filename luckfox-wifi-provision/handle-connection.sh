#!/bin/sh
LOG_FILE=/var/log/luckfox-wifi-provision.log
TTY_PATH="${1:-/dev/rfcomm0}"
printf '%s handler starting on %s\n' "$(date -Iseconds 2>/dev/null || date)" "$TTY_PATH" >> "$LOG_FILE"
exec /usr/bin/python3 /opt/luckfox-wifi-provision/provision.py "$TTY_PATH" >> "$LOG_FILE" 2>&1
