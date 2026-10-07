#!/bin/sh

ENV_FILE=/root/luckfox-livekit.env
CLIENT=/root/luckfox-livekit-client
LOG_FILE=/root/luckfox-livekit.log

child_pid=""

stop_child() {
    if [ -n "$child_pid" ]; then
        kill "$child_pid" 2>/dev/null
        wait "$child_pid" 2>/dev/null
    fi
    exit 0
}

trap stop_child INT TERM

set -a
. "$ENV_FILE"
set +a

while true; do
    LIVEKIT_ROOM="luckfox-voice-$(date +%s)"
    export LIVEKIT_ROOM
    "$CLIENT" >>"$LOG_FILE" 2>&1 &
    child_pid=$!
    wait "$child_pid"
    child_pid=""
    sleep 5
done
