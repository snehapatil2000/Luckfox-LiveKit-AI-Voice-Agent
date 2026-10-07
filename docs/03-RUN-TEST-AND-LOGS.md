# Run, Test, and Show Logs

## Start the full PC demonstration

Open PowerShell:

```powershell
cd "C:\Users\sneha\OneDrive\Documents\PROJECTS\AI VOICE AGENT\Livekit-Deepseek-Deepgram\Luckfox-LiveKit-AI-Voice-Agent-With-PC\scripts"
powershell -ExecutionPolicy Bypass -File .\run_with_pc_demo.ps1
```

The default values are:

```text
BoardIP: 192.168.0.106
AgentDirectory: C:\Users\sneha\Downloads\agent-server
AgentName: my-agent-local
```

Override the board IP if DHCP changes it:

```powershell
powershell -ExecutionPolicy Bypass -File .\run_with_pc_demo.ps1 -BoardIP 192.168.0.120
```

## Correct startup logs

```text
starting worker
registered worker ... "agent_name": "my-agent-local"
received job request
Established new Deepgram STT WebSocket connection
Established new Deepgram TTS WebSocket connection
VOICE STATE: listening
```

## Voice tests

Test wake-word blocking:

```text
What is the capital of India?
```

Expected: no answer. Log:

```text
WAKE WORD: ignored speech without Agent
```

Test normal AI:

```text
Agent, what is the capital of India?
```

Expected:

```text
VOICE USER: what is the capital of India?
VOICE STATE: thinking
VOICE STATE: speaking
VOICE ASSISTANT: The capital of India is New Delhi.
VOICE STATE: listening
```

Test the clock:

```text
Agent, what time is it?
```

Expected:

```text
CLOCK: The current time in India is ...
VOICE ASSISTANT: The current time in India is ...
```

Test two-step wake mode:

```text
Agent
```

Expected answer: `Yes, I'm listening.` Ask the command within ten seconds.

## Open board logs in another PowerShell window

```powershell
cd "C:\Users\sneha\OneDrive\Documents\PROJECTS\AI VOICE AGENT\Livekit-Deepseek-Deepgram\Luckfox-LiveKit-AI-Voice-Agent-With-PC\scripts"
powershell -ExecutionPolicy Bypass -File .\show_luckfox_logs.ps1
```

Stop continuous logs with `Ctrl+C`.

## Development tmux logs

When using the existing WSL tmux session:

```bash
tmux attach -t 0
```

The PC agent is shown in:

```text
pane 0:0.1
```

Capture recent output without attaching:

```bash
tmux capture-pane -p -t 0:0.1 -S -120
```

The reusable PowerShell script is easier for a future instructor demonstration and does not require tmux.

## Stop

Press `Ctrl+C` in the PowerShell window running `run_with_pc_demo.ps1`.

The board service may remain connected and wait for `my-agent-local`. To stop it too:

```powershell
ssh root@192.168.0.106 "/etc/init.d/S99zz-livekit-voice stop"
```

## Restart only the existing board service

No build or deployment is required:

```powershell
cd "C:\Users\sneha\OneDrive\Documents\PROJECTS\AI VOICE AGENT\Livekit-Deepseek-Deepgram\Luckfox-LiveKit-AI-Voice-Agent-With-PC\scripts"
powershell -ExecutionPolicy Bypass -File .\restart_luckfox_voice.ps1
```

