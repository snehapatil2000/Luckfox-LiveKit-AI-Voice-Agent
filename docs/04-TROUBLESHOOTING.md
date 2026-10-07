# Troubleshooting

Run these checks in order.

## No voice answer

Every question must begin clearly with `Hey Siri`. Check that the board is
connected and the remote audio track is subscribed:

```sh
pidof luckfox-livekit-client
tail -n 80 /root/luckfox-livekit.log
```

Look for `Connected`, `Microphone is live`, and `Receiving voice from`.

## Wake phrase is ignored

Speak close to the board microphone and use one sentence:

```text
Hey Siri, what is the time?
```

Background noise, clipping, distance, or STT transcription errors can remove
the wake phrase. Cloud logs show the transcript that the gate received.

## Reply stops or repeats

The speaker may be feeding audio back into the nearby microphone. The agent
contains echo rejection and disables ordinary acoustic interruption. Keep the
speaker volume moderate, move it away from the microphone, or use headphones.
Only an explicit `Hey Siri` command should interrupt a response.

## Slow response

Compare cloud timestamps for `VOICE USER`, `thinking`, the Sarvam WebSocket,
and `speaking`. Normal processing includes STT endpointing, DeepSeek, Sarvam
TTS, and network transport. Delays of tens of seconds usually indicate an
unstable network or a provider reconnect rather than local CPU processing.

## DNS timeout or no route

```sh
wpa_cli -i wlan0 status
ip route
cat /etc/resolv.conf
ping -I wlan0 -c 2 8.8.8.8
```

Renew Wi-Fi DHCP if needed:

```sh
udhcpc -i wlan0 -n -q -t 10
```

Use only one working default route. If Ethernet has no internet but owns the
default route, disconnect it or remove that route. DNS lookup must work before
restarting the voice service.

## Sarvam credits exhausted

Cloud logs show close code `1003` and `Credits exhausted`. Add Sarvam credits,
confirm `SARVAM_API_KEY`, and restart or redeploy the cloud worker.

## Cloud deployment inactive

`The agent has shut down due to inactivity` is expected when no board room is
active. Restart the board service, wait for a new room, and reconnect the log
viewer.

## ADB offline

```powershell
adb kill-server
adb start-server
adb devices
```

ADB is not needed for normal voice operation. Use Ethernet/SSH if USB ADB is
temporarily unavailable.

## Check board configuration

```sh
grep -E '^(LIVEKIT_URL|LIVEKIT_AGENT_NAME|CAPTURE_DEVICE|PLAYBACK_DEVICE)=' /root/luckfox-livekit.env
```

The cloud worker name must be `my-agent`. Do not print the API secret.

## Validate Python changes

```powershell
cd reference-code\agent-server
uv run python -m py_compile src\agent.py
uv run ruff check src\agent.py tests
uv run pytest
```

## Harmless board warnings

IPv6/mDNS and early ICE candidate warnings are normally harmless if later
lines show `Connected`.
