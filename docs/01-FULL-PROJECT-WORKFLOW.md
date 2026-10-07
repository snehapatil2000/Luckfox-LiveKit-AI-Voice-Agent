# Complete Project Workflow — With Windows PC

## 1. Project goal

The goal is to speak into the microphone connected to the Luckfox Pico Pi A W and hear an AI-generated voice answer from the Luckfox speaker or headphones.

The system uses live audio streaming. It does not wait for a complete WAV file before processing.

## 2. Tested architecture

```text
                            INTERNET / CLOUD

Luckfox microphone
       |
       | live PCM audio encoded as Opus
       v
LiveKit Cloud room
       |
       | sends the microphone track to my-agent-local
       v
Windows PC: Python LiveKit agent
       |
       +----> Deepgram Nova-3 STT
       |          speech -> text
       |
       +----> DeepSeek V4 Flash LLM
       |          question -> answer
       |
       +----> Deepgram Aura-2 TTS
                  text -> speech
       |
       v
LiveKit Cloud room
       |
       | live Opus audio
       v
Luckfox speaker / headphones
```

## 3. What each component does

| Component | Responsibility |
|---|---|
| Luckfox Pico Pi A W | Reads the microphone, publishes audio, receives reply audio, and plays it through ALSA. |
| LiveKit Cloud | Real-time communication server. It creates the room and transports audio in both directions. |
| Windows PC | Runs the Python orchestration agent named `my-agent-local` and displays voice logs. |
| Deepgram Nova-3 | Speech-to-text (STT). Converts the user's voice into text. |
| DeepSeek V4 Flash | Large language model (LLM). Understands the text and generates an answer. |
| Deepgram Aura-2 | Text-to-speech (TTS). Converts the answer into voice. |

This is an internet/cloud project **with a local PC agent**. It is not offline AI.

## 4. Current tested values

| Setting | Value |
|---|---|
| Board | Luckfox Pico Pi A W |
| Board IP | `192.168.0.106` |
| SSH user | `root` |
| CPU | ARMv7 (`armv7l`) |
| C library | uClibc (`libc.so.0`) |
| Capture device | ALSA `hw:0,0` |
| Playback device | ALSA `hw:0,0` |
| Audio rate | 48 kHz |
| PC agent name | `my-agent-local` |
| STT model | Deepgram `nova-3`, `en-US` |
| LLM model | DeepSeek `deepseek-v4-flash` |
| TTS model | Deepgram `aura-2-andromeda-en` |
| Wake word | `Agent` |
| Wake window | 10 seconds after saying only `Agent` |
| Clock timezone | India Standard Time (UTC+05:30) |

The private IP can change after a router or board restart. Replace it in commands when necessary.

## 5. Audio and AI sequence

1. The Luckfox runs `/root/luckfox-livekit-client`.
2. The client creates a new room named `luckfox-voice-<timestamp>`.
3. The client dispatches the PC worker named `my-agent-local`.
4. `arecord` captures stereo 48 kHz PCM audio from `hw:0,0`.
5. The Go client mixes the stereo microphone into mono and publishes it to LiveKit.
6. The Windows Python agent receives the microphone track.
7. Deepgram transcribes the stream.
8. The Python wake gate checks the final transcript.
9. Speech without `Agent` at the beginning is ignored.
10. A valid command is sent to DeepSeek, except clock questions, which are answered directly using India time.
11. Deepgram TTS generates reply audio.
12. LiveKit publishes the reply track to the Luckfox.
13. The Go client decodes it and writes PCM audio to `aplay` on `hw:0,0`.

## 6. Wake-word behavior

Supported one-sentence command:

```text
Agent, what is the capital of India?
```

Supported two-step command:

```text
User: Agent
Assistant: Yes, I'm listening.
User within 10 seconds: What time is it?
```

Without the wake word:

```text
What is the capital of India?
```

The agent does not send that command to DeepSeek and does not answer.

Deepgram still performs transcription because the server-side Python program needs the transcript to recognize the wake word. A fully private on-device wake word would require a separate detector on the Luckfox.

## 7. Clock behavior

Clock requests are handled directly by Python, using fixed India Standard Time. This avoids an LLM guessing the time.

Example:

```text
Agent, what time is it?
```

Expected answer format:

```text
The current time in India is 3:45 PM on Thursday, 10 September 2026.
```

## 8. Credential flow

The Windows file `.env.local` contains:

```dotenv
LIVEKIT_URL=wss://your-project.livekit.cloud
LIVEKIT_API_KEY=your_livekit_api_key
LIVEKIT_API_SECRET=your_livekit_api_secret
DEEPSEEK_API_KEY=your_deepseek_api_key
DEEPGRAM_API_KEY=your_deepgram_api_key
```

The Luckfox prototype file `/root/luckfox-livekit.env` contains only the LiveKit connection credentials and audio settings. DeepSeek and Deepgram keys stay on the Windows PC.

Never commit `.env.local` or a real board environment file to Git.

## 9. PC software requirements

Windows requires:

- PowerShell
- Python 3.10 through 3.14
- `uv`
- OpenSSH client (`ssh` and `scp`)
- WSL Ubuntu for rebuilding the ARM client
- Luckfox ARM uClibc cross-toolchain
- Go toolchain used by the supplied build script

Verify in PowerShell:

```powershell
python --version
uv --version
ssh -V
wsl --status
```

## 10. One-time Python agent installation

Place the agent at:

```text
C:\Users\sneha\Downloads\agent-server
```

Then run:

```powershell
cd "$env:USERPROFILE\Downloads\agent-server"
Copy-Item .env.example .env.local
notepad .env.local
uv sync
uv run -m livekit.agents download-files
uv run python -m py_compile src\agent.py
```

Fill in real keys before starting the agent. Do not share the values.

## 11. Fast daily demonstration

Open PowerShell:

```powershell
cd "C:\Users\sneha\OneDrive\Documents\PROJECTS\AI VOICE AGENT\Livekit-Deepseek-Deepgram\Luckfox-LiveKit-AI-Voice-Agent-With-PC"
powershell -ExecutionPolicy Bypass -File .\START-WITH-PC-DEMO.ps1
```

The script changes the board to `my-agent-local`, restarts its existing audio service, and runs the Python agent in the current PowerShell window.

Wait for:

```text
registered worker ... "agent_name": "my-agent-local"
received job request ...
VOICE STATE: listening
```

Speak near the Luckfox, not near the PC microphone.

## 12. What must stay on

During PC mode, all of these must remain available:

- Luckfox power
- Luckfox network connection
- Windows PC power
- The PowerShell Python-agent process
- Internet access
- LiveKit Cloud
- Deepgram account and credit
- DeepSeek account and credit

Closing the agent PowerShell window or switching off the PC stops AI answers.
