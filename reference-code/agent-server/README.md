# Luckfox Python Voice Agent

This directory contains the Python worker deployed to LiveKit Cloud for the
Luckfox voice assistant.

## Pipeline

```text
Luckfox audio -> LiveKit -> Sarvam Saaras v4 -> DeepSeek V4 Flash
              <- LiveKit <- Sarvam Bulbul v3 <- generated answer
```

The agent supports English and Kannada. Each question must start with **Hey
Siri**. An explicit Hey Siri command can interrupt a reply; ordinary speech
and speaker echo cannot.

## Setup

```powershell
Copy-Item .env.example .env.local
uv sync --locked
uv run pytest
uv run src/agent.py dev
```

Set these values in the untracked `.env.local` file:

```dotenv
LIVEKIT_URL=wss://your-project.livekit.cloud
LIVEKIT_API_KEY=your_livekit_api_key
LIVEKIT_API_SECRET=your_livekit_api_secret
DEEPSEEK_API_KEY=your_deepseek_api_key
SARVAM_API_KEY=your_sarvam_api_key
```

`DEEPGRAM_API_KEY` is optional and is used only by the English fallback when
Sarvam is not configured. Kannada requires Sarvam.

## Production

Authenticate and deploy with the LiveKit CLI from this directory. The
deployed worker name defaults to `my-agent`; override it with
`LIVEKIT_AGENT_NAME` only when deliberately using a second worker.

Follow production logs with:

```powershell
lk agent logs --id <cloud-agent-id> --project <project-name> --log-type deploy
```

Useful log messages are `VOICE USER`, `VOICE ASSISTANT`, `VOICE STATE`,
`WAKE WORD`, `WAKE BARGE-IN`, `TOOL CALL`, and `TOOL RESULT`.

Never commit `.env.local` or copy cloud-provider secrets into source files.
