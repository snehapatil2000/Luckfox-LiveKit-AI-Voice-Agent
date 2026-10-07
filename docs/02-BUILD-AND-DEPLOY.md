# Build and Deploy

Use this guide for a new board or after changing the Go audio client. Replace
`<board-ip>` with the board's current DHCP address; do not hard-code a private
address because it can change after rebooting or changing networks.

## Check the board

```powershell
ssh root@<board-ip>
```

```sh
uname -m
arecord -l
aplay -l
ip address
ip route
cat /etc/resolv.conf
```

The supplied binary targets ARMv7 with uClibc. The tested capture and playback
device is `hw:0,0` at 48 kHz.

## Test audio before deployment

```sh
/etc/init.d/S99zz-livekit-voice stop 2>/dev/null || true
arecord -D hw:0,0 -f S16_LE -r 48000 -c 2 -d 5 /tmp/mic-test.wav
aplay -D hw:0,0 /tmp/mic-test.wav
```

## Build the ARM client

From WSL at the repository root:

```bash
chmod +x scripts/build_luckfox_client.sh
./scripts/build_luckfox_client.sh
```

Output:

```text
reference-code/luckfox-client/build/luckfox-livekit-client
```

The script supports `GO_BIN` and `CROSS_PREFIX` environment overrides when
the toolchains are installed elsewhere.

## Deploy to Luckfox

From Windows PowerShell at the repository root:

```powershell
.\DEPLOY-FRESH-LUCKFOX.ps1 -BoardIP <board-ip>
```

This installs the voice client, startup service, environment template, and
experimental Bluetooth provisioning files. Existing board credentials are
not overwritten.

## Configure the board once

```sh
cp /root/luckfox-livekit.env.example /root/luckfox-livekit.env
vi /root/luckfox-livekit.env
```

Use:

```dotenv
LIVEKIT_URL=wss://your-project.livekit.cloud
LIVEKIT_API_KEY=your_livekit_api_key
LIVEKIT_API_SECRET=your_livekit_api_secret
LIVEKIT_AGENT_NAME=my-agent
CAPTURE_DEVICE=hw:0,0
PLAYBACK_DEVICE=hw:0,0
```

Then protect and restart it:

```sh
chmod 600 /root/luckfox-livekit.env
/etc/init.d/S99zz-livekit-voice restart
tail -n 60 /root/luckfox-livekit.log
```

Expected output includes:

```text
Connected to LiveKit room luckfox-voice-...
Dispatched agent my-agent
Microphone is live. Speak near the Luckfox board.
Receiving voice from agent-...
```

## Configure and test the Python agent

```powershell
cd reference-code\agent-server
Copy-Item .env.example .env.local
uv sync --locked
uv run pytest
uv run src/agent.py dev
```

For production, configure the same secrets in LiveKit Cloud and deploy the
agent from this directory using the LiveKit CLI. Do not put Sarvam or
DeepSeek credentials on the board.

## Reboot test

```sh
reboot
```

After the board returns:

```sh
pidof luckfox-livekit-client
tail -n 40 /root/luckfox-livekit.log
```

The PC and ADB may then be disconnected. Keep board power, Wi-Fi, and internet
available.
