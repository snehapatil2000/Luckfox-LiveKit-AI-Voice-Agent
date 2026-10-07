# Run, Test, and Show Logs

## Start or restart the board client

Using ADB:

```powershell
adb devices
adb -s <board-serial> shell /etc/init.d/S99zz-livekit-voice restart
adb -s <board-serial> shell "tail -f /root/luckfox-livekit.log"
```

Using SSH:

```powershell
ssh root@<board-ip> "/etc/init.d/S99zz-livekit-voice restart"
ssh root@<board-ip> "tail -f /root/luckfox-livekit.log"
```

Board logs prove transport and audio state. They do not show full user and
assistant transcripts.

## Follow LiveKit Cloud logs

```powershell
lk agent logs --id <cloud-agent-id> --project <project-name> --log-type deploy
```

Useful entries:

```text
VOICE USER: ...
VOICE ASSISTANT: ...
VOICE STATE: listening|thinking|speaking
WAKE WORD: ...
WAKE BARGE-IN: ...
TOOL CALL: ...
TOOL RESULT: ...
Starting TTS WebSocket session
WebSocket session completed successfully
```

`The agent has shut down due to inactivity` means there is no active room.
Restart the board voice service to create a room, then run the log command
again. `scanner error` is a log-stream connection failure; rerun the command.

## WSL tmux pane

If logs are displayed in pane `0:0.1`:

```bash
tmux capture-pane -p -t 0:0.1 -S -120
tmux attach -t 0
```

The pane normally runs the LiveKit CLI command above, not ADB. Use ADB only
for `/root/luckfox-livekit.log`.

## Voice tests

English:

```text
Hey Siri, what is the capital of India?
Hey Siri, what time is it?
Hey Siri, what is the weather in Bengaluru?
```

Kannada:

```text
Hey Siri, ಕರ್ನಾಟಕದ ಜಿಲ್ಲೆಗಳನ್ನು ಹೇಳು.
Hey Siri, ಭಾರತದ ರಾಜ್ಯಗಳನ್ನು ಹೇಳು.
```

Wake-word rejection:

```text
What is the time?
```

Expected: no answer and `WAKE WORD: ignored speech without Hey Siri`.

Barge-in:

1. Ask for a story or a list.
2. While the assistant speaks, say `Hey Siri, what is the time?`.
3. The previous audio should stop and the new answer should begin.

## Stop continuous logs

Press `Ctrl+C`. This stops only the log viewer; it does not stop the board or
cloud agent.
