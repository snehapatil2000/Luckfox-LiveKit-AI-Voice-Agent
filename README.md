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
                                             |
Luckfox headphones/speaker <- LiveKit Cloud <-+
```

- English: Sarvam Saaras STT -> DeepSeek -> Sarvam Bulbul `en-IN` TTS
- Kannada: Sarvam Saaras STT -> DeepSeek -> Sarvam Bulbul `kn-IN` TTS
- Wake phrase: **Hey Siri** at the start of every question
- Barge-in: say **Hey Siri** plus a new question while an answer is playing
- Tools: calculator, current time/date, live weather, and project information
- Startup: `/etc/init.d/S99zz-livekit-voice` starts the board client at boot

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

The tracked ARM client is:

```text
reference-code/luckfox-client/build/luckfox-livekit-client
```

From the repository root in PowerShell:

```powershell
.\DEPLOY-FRESH-LUCKFOX.ps1 -BoardIP <board-ip>
```

Configure `/root/luckfox-livekit.env` on the board, set
`LIVEKIT_AGENT_NAME=my-agent`, protect it with `chmod 600`, and restart:

```sh
/etc/init.d/S99zz-livekit-voice restart
tail -f /root/luckfox-livekit.log
```

Expected lines include `Connected to LiveKit room`, `Dispatched agent
my-agent`, `Microphone is live`, and `Receiving voice from agent`.

## Test

Ask near the board:

```text
Hey Siri, what time is it?
Hey Siri, tell me the districts of Karnataka.
Hey Siri, ಕರ್ನಾಟಕದ ಜಿಲ್ಲೆಗಳನ್ನು ಹೇಳು.
```

Every independent question must include **Hey Siri**. During a long answer,
say **Hey Siri** followed by the next question to stop the old reply and start
the new one.

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

## Security

Real API keys and `.env.local` are intentionally excluded. Never commit
LiveKit secrets, DeepSeek keys, Sarvam keys, private keys, or board runtime
environment files. The board currently stores LiveKit credentials for this
prototype; production hardware should obtain short-lived tokens from a
trusted token service.

See [docs/01-FULL-PROJECT-WORKFLOW.md](docs/01-FULL-PROJECT-WORKFLOW.md) for
the complete flow and [docs/05-INSTRUCTOR-DEMO.md](docs/05-INSTRUCTOR-DEMO.md)
for a short demonstration script.
