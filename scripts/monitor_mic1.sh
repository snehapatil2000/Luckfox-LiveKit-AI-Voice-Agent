#!/usr/bin/env bash

set -u

plink="${PLINK_BIN:-/mnt/c/Program Files/PuTTY/plink.exe}"
host="${LUCKFOX_HOST:-192.168.0.101}"
fingerprint="${LUCKFOX_HOSTKEY:-SHA256:LDYwx4hY/HbfCW14ADyTMQbZcksii11n1wLaNaNMds8}"

while true; do
    "$plink" -batch -ssh -hostkey "$fingerprint" -pw luckfox "root@$host" '
        while true; do
            timestamp=$(date +%H:%M:%S)
            status=/proc/asound/card0/pcm0c/sub0/status
            state=$(sed -n "s/^state: //p" "$status" 2>/dev/null)
            owner=$(sed -n "s/^owner_pid[[:space:]]*:[[:space:]]*//p" "$status" 2>/dev/null)
            if [ "$state" = RUNNING ]; then
                echo "$timestamp MIC1=ON CAPTURE=RUNNING PID=${owner:-unknown} DEVICE=hw:0,0"
            else
                echo "$timestamp MIC1=OFF CAPTURE=${state:-CLOSED} DEVICE=hw:0,0"
            fi
            sleep 1
        done
    '
    printf '%s MIC1=UNKNOWN BOARD=OFFLINE; retrying SSH\n' "$(date +%H:%M:%S)"
    sleep 2
done
