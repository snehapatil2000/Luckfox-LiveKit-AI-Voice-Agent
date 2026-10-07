# Luckfox LiveKit AI Voice Agent

This repository contains the complete Luckfox Pico Pi AI voice-agent project.
The verified voice path runs without a PC after the board has been configured:

```text
Luckfox microphone
    -> LiveKit Cloud
    -> Python agent
    -> Sarvam Saaras STT
    -> DeepSeek LLM
    -> Sarvam Bulbul TTS
    -> LiveKit Cloud
    -> Luckfox speaker or headphones
```

The agent supports English and Kannada, uses **Hey Siri** as the wake phrase,
and keeps the board voice client running automatically at boot. The PC is used
for development, deployment, and viewing logs; it is not required during the
normal board-only demonstration. Wi-Fi internet and board power are required.

## Main project folders

```text
reference-code/agent-server/       Python LiveKit Cloud agent
reference-code/luckfox-client/     ARM client source and deployable binary
luckfox-wifi-provision/            Bluetooth Wi-Fi provisioning service
luckfox-wifi-provision-app/        Android provisioning app source
docs/                              Build, deployment, demo, and security notes
DEPLOY-FRESH-LUCKFOX.ps1           Fresh-board deployment script
```

## Cloud agent

Copy `reference-code/agent-server/.env.example` to a local `.env.local` and
fill in the LiveKit, DeepSeek, and Sarvam credentials. Never commit `.env.local`
or API keys.

```powershell
cd reference-code\agent-server
uv sync --locked
uv run python src\agent.py dev
```

The production agent can also be deployed with the LiveKit CLI. The tracked
`uv.lock` file keeps the dependency versions reproducible.

## Fresh Luckfox deployment

The deployable ARM client is kept at:

```text
reference-code/luckfox-client/build/luckfox-livekit-client
```

From PowerShell:

```powershell
powershell -ExecutionPolicy Bypass -File .\DEPLOY-FRESH-LUCKFOX.ps1 -BoardIP <board-ip>
```

The board services are:

```text
/etc/init.d/S99zz-livekit-voice
/etc/init.d/S98wifi-provision
```

Bluetooth Wi-Fi provisioning files are included for the later provisioning
step. The confirmed voice-agent path uses the board's configured Wi-Fi.

## Security

No real API keys are included in this repository. Keep all runtime credentials
in local environment files or the board's protected runtime configuration.
