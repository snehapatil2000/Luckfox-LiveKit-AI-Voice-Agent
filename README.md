# Luckfox LiveKit AI Voice Agent

A bilingual English/Kannada voice assistant for the Luckfox Pico Pi A W.
The board captures and plays audio while the Python agent runs in LiveKit
Cloud, so normal operation needs only board power, audio hardware, Wi-Fi, and
internet access. A PC is needed for development, deployment, and logs—not for
the live voice conversation.

Repository: <https://github.com/snehapatil2000/Luckfox-LiveKit-AI-Voice-Agent>

## Architecture

```text
User -> Luckfox microphone -> LiveKit Cloud -> Python agent
                                             |
                                             +-> Sarvam Saaras v4 STT
                                             +-> DeepSeek V4 Flash LLM
                                             +-> Sarvam Bulbul v3 TTS
                                             +-> GeckoLink tool request
                                             |
Luckfox headphones/speaker <- LiveKit Cloud <-+

GeckoLink tool request -> LiveKit data -> Luckfox -> A20 GeckoLink REST API
                         -> coordinator -> Zigbee switch/light
```

- English: Sarvam Saaras STT -> DeepSeek -> Sarvam Bulbul `en-IN` TTS
- Kannada: Sarvam Saaras STT -> DeepSeek -> Sarvam Bulbul `kn-IN` TTS
- Activation: say “Alexa” to the local microWakeWord detector on MIC1
- Feedback: the LED lights and a beep plays, then the LED switches off
- Question: begin within 1.5 seconds after the beep. A 1.5-second pause ends the question
- Barge-in: say “Alexa” during an answer, wait for the beep, then ask the next question
- Tools: calculator, time/date, weather, project information, and GeckoLink control
- Startup: `/etc/init.d/S99zz-livekit-voice` starts the board client at boot

The existing GeckoLink mobile-app path is unchanged. Users may control the
same points either from the app or by voice. Voice control reuses GeckoLink's
authenticated API and its existing point, room, scene, mood, coordinator, and
Zigbee logic.

## Repository layout

```text
reference-code/agent-server/       Python LiveKit agent and tests
reference-code/luckfox-client/     Go audio client, ARM binary, board service
luckfox-wifi-provision/            Experimental Bluetooth provisioning service
luckfox-wifi-provision-app/        Experimental Android provisioning app
docs/                              Setup, logs, troubleshooting, and demo notes
DEPLOY-FRESH-LUCKFOX.ps1           Complete fresh-board deployment
```

Bluetooth Wi-Fi provisioning is included but is not part of the confirmed
voice demo. Configure Wi-Fi on the board directly until that work is resumed.

## Requirements

- Luckfox Pico Pi A W with microphone and speaker/headphones
- 2.4 GHz Wi-Fi with internet access
- LiveKit Cloud project and deployed agent named `my-agent`
- LiveKit, DeepSeek, and Sarvam API credentials
- Windows/WSL only for building, deploying, and monitoring

## Configure the cloud agent

```powershell
cd reference-code\agent-server
Copy-Item .env.example .env.local
notepad .env.local
uv sync --locked
uv run pytest
```

Required values in `.env.local`:

```dotenv
LIVEKIT_URL=wss://your-project.livekit.cloud
LIVEKIT_API_KEY=your_livekit_api_key
LIVEKIT_API_SECRET=your_livekit_api_secret
DEEPSEEK_API_KEY=your_deepseek_api_key
SARVAM_API_KEY=your_sarvam_api_key
```

Run locally with `uv run src/agent.py dev`, or deploy from this directory with
the LiveKit CLI. Cloud credentials must be configured as deployment secrets.

## Deploy the Luckfox client

Install the microWakeWord runtime with the updated client.
See [Alexa setup and recovery](docs/MICROWAKEWORD.md) for build, installation, and test commands.
The runtime is a required dependency. The general deployment script does not install it.

The tracked ARM client is:

```text
reference-code/luckfox-client/build/luckfox-livekit-client
```

From the repository root in PowerShell:

```powershell
.\DEPLOY-FRESH-LUCKFOX.ps1 -BoardIP <board-ip>
```

Configure `/root/luckfox-livekit.env` on the board, set
`LIVEKIT_AGENT_NAME=my-agent` and `GECKOLINK_USERNAME` /
`GECKOLINK_PASSWORD` to a dedicated GeckoLink user account. The client identifies
the A20 by its fixed MAC `30:1F:9A:D1:45:49` and refreshes its current LAN IP
automatically; no gateway IP is configured. Protect the file
with `chmod 600`. The client logs in again automatically when its token expires;
`GECKOLINK_TOKEN` remains available for static-token deployments.

The token needs only `points` read/control, `rooms` read, `scenes`
read/control, and `moods` read/control permissions. Do not use an admin token.

Restart the client:

```sh
/etc/init.d/S99zz-livekit-voice restart
tail -f /root/luckfox-livekit.log
```

Expected lines include `Connected to LiveKit room`, `Dispatched agent
my-agent`, `WAKE READY`, and `Receiving voice from agent`.

## Test

Say “Alexa.” After the beep and LED switch off, ask near the board:

```text
What time is it?
Turn on the living room ceiling light.
ಲಿವಿಂಗ್ ರೂಮ್ ಲೈಟ್ ಆನ್ ಮಾಡಿ.
```

Each independent question needs “Alexa.” During a long answer, say “Alexa”
again. After the beep, ask the next question.
MIC1 stays open for local detection. Only question audio goes to the cloud.

## Logs

Board transport/audio logs:

```powershell
adb -s <board-serial> shell "tail -f /root/luckfox-livekit.log"
```

Cloud transcript and state logs:

```powershell
lk agent logs --id <cloud-agent-id> --project <project-name> --log-type deploy
```

Cloud logs show `VOICE USER`, `VOICE ASSISTANT`, `VOICE STATE`, tool calls,
wake-word decisions, and Sarvam WebSocket status. Board logs do not contain
the complete speech transcript.

## GeckoLink voice control

The cloud agent cannot directly reach the A20 board's private LAN address.
For an automation tool call, it sends a structured request over the existing
LiveKit data channel. The Luckfox client checks the request against a strict
allowlist, adds the bearer token locally, calls the A20 GeckoLink REST API,
and returns the result. The token never enters the LLM prompt.

Allowed voice operations are point discovery/status, on/off/toggle,
brightness from 0 to 100, and activation of existing scenes and moods.
Pairing, deletion, renaming, and account administration remain app-only.

## Security

Real API keys and `.env.local` are intentionally excluded. Never commit
LiveKit secrets, DeepSeek keys, Sarvam keys, private keys, or board runtime
environment files. The board currently stores LiveKit credentials for this
prototype; production hardware should obtain short-lived tokens from a
trusted token service.

See [docs/01-FULL-PROJECT-WORKFLOW.md](docs/01-FULL-PROJECT-WORKFLOW.md) for
the complete flow and [docs/05-INSTRUCTOR-DEMO.md](docs/05-INSTRUCTOR-DEMO.md)
for a short demonstration script.
