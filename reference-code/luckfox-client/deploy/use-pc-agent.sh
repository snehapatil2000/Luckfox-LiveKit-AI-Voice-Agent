#!/bin/sh

ENV_FILE=/root/luckfox-livekit.env

sed -i '/^LIVEKIT_AGENT_NAME=/d' "$ENV_FILE"
echo 'LIVEKIT_AGENT_NAME=my-agent-local' >>"$ENV_FILE"
chmod 600 "$ENV_FILE"

/etc/init.d/S99zz-livekit-voice restart
echo "Luckfox is now using the Windows PC agent: my-agent-local"
