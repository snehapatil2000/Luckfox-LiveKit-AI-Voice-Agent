# Build and Deploy the Luckfox Client

Use this document for a new Luckfox board or when rebuilding the Go audio client. For an already-installed demonstration, skip this document and run `scripts/run_with_pc_demo.ps1`.

## 1. Verify the board

From PowerShell:

```powershell
ssh root@192.168.0.106
```

Run on Luckfox:

```sh
uname -m
python3 --version
ldd /bin/sh 2>&1 | head -1
arecord -l
aplay -l
df -h
```

Expected important values:

```text
armv7l
libc.so.0 => /lib/libc.so.0
capture card 0, device 0
playback card 0, device 0
```

## 2. Test the board audio first

Stop the LiveKit client before taking the ALSA device:

```sh
/etc/init.d/S99zz-livekit-voice stop 2>/dev/null || true
arecord -D hw:0,0 -f S16_LE -r 48000 -c 2 -d 5 /tmp/mic-test.wav
aplay -D hw:0,0 /tmp/mic-test.wav
```

Speak during the five-second recording. Continue only if your voice plays correctly.

## 3. Prepare WSL build requirements

The tested build uses:

```text
Go: /home/sneha/agent/toolchains/go/bin/go
Cross GCC prefix:
/home/sneha/luckfox-pico-sdk/tools/linux/toolchain/
arm-rockchip830-linux-uclibcgnueabihf/bin/
arm-rockchip830-linux-uclibcgnueabihf-
```

Required Ubuntu utilities include `curl`, `tar`, `make`, `patch`, and `pkg-config`.

The build script downloads and builds Opus 1.5.2 if the static ARM library is missing. It then downloads Go modules, creates `vendor`, applies `livekit-armv7.patch`, and builds an ARMv7/uClibc binary.

## 4. Run the build script

From WSL:

```bash
cd "/mnt/c/Users/sneha/OneDrive/Documents/PROJECTS/AI VOICE AGENT/Livekit-Deepseek-Deepgram/Luckfox-LiveKit-AI-Voice-Agent-With-PC"
chmod +x scripts/build_luckfox_client.sh
./scripts/build_luckfox_client.sh
```

Optional path overrides:

```bash
GO_BIN=/path/to/go \
CROSS_PREFIX=/path/to/arm-rockchip830-linux-uclibgnueabihf- \
./scripts/build_luckfox_client.sh
```

Output:

```text
reference-code/luckfox-client/build/luckfox-livekit-client
```

Expected `file` result:

```text
ELF 32-bit LSB executable, ARM, EABI5, interpreter /lib/ld-uClibc.so.0
```

## 5. Deploy to a new board

Open Windows PowerShell:

```powershell
cd "C:\Users\sneha\OneDrive\Documents\PROJECTS\AI VOICE AGENT\Livekit-Deepseek-Deepgram\Luckfox-LiveKit-AI-Voice-Agent-With-PC\scripts"
powershell -ExecutionPolicy Bypass -File .\deploy_luckfox_with_pc.ps1 -BoardIP 192.168.0.106
```

The script copies:

```text
luckfox-livekit-client  -> /root/luckfox-livekit-client
run-luckfox-livekit.sh  -> /root/run-luckfox-livekit.sh
use-pc-agent.sh         -> /root/use-pc-agent.sh
S99zz-livekit-voice     -> /etc/init.d/S99zz-livekit-voice
environment template    -> /root/luckfox-livekit.env.example
```

The script does not overwrite an existing `/root/luckfox-livekit.env` because it may contain private credentials.

## 6. Configure a new board once

If the deployment says the board environment is missing:

```powershell
ssh root@192.168.0.106
```

On Luckfox:

```sh
cp /root/luckfox-livekit.env.example /root/luckfox-livekit.env
vi /root/luckfox-livekit.env
```

Use:

```dotenv
LIVEKIT_URL=wss://your-project.livekit.cloud
LIVEKIT_API_KEY=your_livekit_api_key
LIVEKIT_API_SECRET=your_livekit_api_secret
LIVEKIT_AGENT_NAME=my-agent-local
CAPTURE_DEVICE=hw:0,0
PLAYBACK_DEVICE=hw:0,0
```

Then:

```sh
chmod 600 /root/luckfox-livekit.env
/etc/init.d/S99zz-livekit-voice restart
```

Do not put DeepSeek or Deepgram keys on the board.

## 7. Verify deployment

On the board:

```sh
cat /var/run/luckfox-livekit.pid
ps -ef | grep -E 'run-luckfox-livekit|luckfox-livekit-client|arecord' | grep -v grep
grep '^LIVEKIT_AGENT_NAME=' /root/luckfox-livekit.env
tail -n 60 /root/luckfox-livekit.log
```

Expected lines:

```text
Connected to LiveKit room luckfox-voice-...
Dispatched agent my-agent-local
Microphone is live. Speak near the Luckfox board.
Receiving voice from agent-...
```

## 8. Reboot test

```sh
reboot
```

Reconnect after the board starts:

```powershell
ssh root@192.168.0.106
```

Then check:

```sh
ps -ef | grep -E 'luckfox-livekit-client|arecord' | grep -v grep
tail -n 40 /root/luckfox-livekit.log
```

