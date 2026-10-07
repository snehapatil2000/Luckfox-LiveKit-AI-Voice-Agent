# Troubleshooting

Use these checks in order.

## 1. No answer

Confirm the command begins with the wake word:

```text
Agent, what is the capital of India?
```

Speech without `Agent` is intentionally ignored.

## 2. `room session transport is closed`

Example:

```text
RuntimeError: room session transport is closed
job exiting {"reason": "room disconnected"}
```

Meaning:

- The individual LiveKit room disconnected.
- It is normally not a Python crash.
- The main PC worker can still be registered and waiting for another room.
- The Luckfox connection may be stale after a temporary network interruption.

Recovery without rebuilding:

```powershell
cd "C:\Users\sneha\OneDrive\Documents\PROJECTS\AI VOICE AGENT\Livekit-Deepseek-Deepgram\Luckfox-LiveKit-AI-Voice-Agent-With-PC\scripts"
powershell -ExecutionPolicy Bypass -File .\restart_luckfox_voice.ps1
```

Wait for a new `received job request` and `VOICE STATE: listening` line.

## 3. Board service and process checks

```powershell
ssh root@192.168.0.106
```

```sh
cat /var/run/luckfox-livekit.pid
ps -ef | grep -E 'run-luckfox-livekit|luckfox-livekit-client|arecord|aplay' | grep -v grep
tail -n 100 /root/luckfox-livekit.log
```

On this BusyBox board, `pidof luckfox-livekit-client` may not always show the full picture. `ps -ef` is preferred.

## 4. Wrong agent selected

Check:

```sh
grep '^LIVEKIT_AGENT_NAME=' /root/luckfox-livekit.env
```

PC mode must show:

```text
LIVEKIT_AGENT_NAME=my-agent-local
```

Correct it:

```sh
/root/use-pc-agent.sh
```

## 5. Microphone unavailable

Stop the service and test ALSA:

```sh
/etc/init.d/S99zz-livekit-voice stop
arecord -D hw:0,0 -f S16_LE -r 48000 -c 2 -d 5 /tmp/mic-test.wav
aplay -D hw:0,0 /tmp/mic-test.wav
/etc/init.d/S99zz-livekit-voice start
```

Also check for another recorder:

```sh
ps -ef | grep arecord | grep -v grep
```

## 6. Agent registered but no room job

Restart the board service:

```sh
/etc/init.d/S99zz-livekit-voice restart
```

This creates a fresh room and dispatches `my-agent-local` again.

## 7. DeepSeek does not answer

Check:

- `DEEPSEEK_API_KEY` exists in the PC `.env.local`.
- The DeepSeek account has credit.
- The model name is `deepseek-v4-flash`.
- The PC has internet access.

Do not print or share the key.

## 8. Deepgram STT or TTS does not connect

Check:

- `DEEPGRAM_API_KEY` exists in `.env.local`.
- The Deepgram account has credit.
- Logs show both STT and TTS WebSocket connections.
- Models are `nova-3` and `aura-2-andromeda-en`.

## 9. Old Deepgram constructor error

Do not use:

```python
deepgram.TTS(model="aura-2", voice="athena")
```

Use:

```python
deepgram.TTS(model="aura-2-andromeda-en")
```

## 10. Python indentation or syntax error

```powershell
cd "$env:USERPROFILE\Downloads\agent-server"
uv run python -m py_compile src\agent.py
uv run ruff check src\agent.py
```

## 11. Harmless board warnings

These warnings are normally harmless when later logs show `Connected`:

```text
Failed to enable mDNS over IPv6
Failed to ping without candidate pairs
```

The Luckfox kernel does not have IPv6 enabled, but IPv4 connectivity succeeds.

## 12. IP changed

Find the board's new DHCP address and use it:

```powershell
ssh root@NEW_IP
powershell -ExecutionPolicy Bypass -File .\run_with_pc_demo.ps1 -BoardIP NEW_IP
```

