# Alexa on Luckfox

MIC1 has one capture process. The Go client sends standby audio to the native microWakeWord helper.
The helper uses the supplied Alexa model. It does not require a Picovoice account or key.

Say **Alexa**. The client stops any current answer, lights the LED, and plays the beep.
After the beep wait finishes, the LED switches off and question capture starts.
Begin your question within 1.5 seconds. Once speech starts, the question can last longer.
The cloud agent processes the question after 1.5 seconds of silence.

MIC1 remains open for local wake detection, including during AI playback.
The detector receives no beep or question audio.
The detector resumes with synthetic silence to clear its previous audio history.
LiveKit receives question audio only.

## Build

On Ubuntu/WSL, install `g++-arm-linux-gnueabihf`, `curl`, and `python3`.
The client also requires the existing Luckfox toolchain and Go environment.

From the repository root, run:

```sh
./scripts/build_luckfox_microwakeword.sh
./scripts/build_luckfox_client.sh
```

The detector build downloads checksum-verified dependencies.
It uses `pymicro-wakeword 2.5.0` and microfrontend revision `96bd69cfad79aa67697e176570d3dd87052c3def`.
The board uses a private glibc loader directory. The system libraries remain unchanged.
Generated libraries, source archives, and models stay outside Git.

## Install

The board must already have the voice service and its protected credential file.
Replace `BOARD_IP` with the current Luckfox address.

```sh
ssh root@BOARD_IP 'mkdir -p /root/microwakeword-staging'
tar -czf /tmp/microwakeword-bundle.tar.gz --exclude=sources \
  -C reference-code/luckfox-client/build/microwakeword .
scp /tmp/microwakeword-bundle.tar.gz \
  reference-code/luckfox-client/build/luckfox-livekit-client \
  reference-code/luckfox-client/deploy/install-microwakeword.sh \
  root@BOARD_IP:/root/microwakeword-staging/
ssh root@BOARD_IP 'cd /root/microwakeword-staging && gzip -dc microwakeword-bundle.tar.gz | tar -xf -'
ssh root@BOARD_IP 'sh /root/microwakeword-staging/install-microwakeword.sh /root/microwakeword-staging'
```

The installer preserves all LiveKit and GeckoLink credentials.
It saves the previous client and configuration in `/root/wake-detector-archive.XXXXXX`.
It moves old detector installations into that directory, including their calibration recordings.
It sets these values in `/root/luckfox-livekit.env`:

```dotenv
WAKE_WORD=alexa
WAKEWORD_BINARY=/opt/luckfox-microwakeword/run-microwakeword.sh
WAKEWORD_MODEL=/opt/luckfox-microwakeword/alexa.tflite
WAKEWORD_CONFIG=/opt/luckfox-microwakeword/alexa.json
```

The configuration must match the model. Changing `WAKE_WORD` cannot train a new word.
The default probability cutoff is 0.9 across five model outputs.
This score is a detection threshold, not a measured accuracy percentage.
The old `WAKEWORD_THRESHOLD` setting does not apply.

## Test and monitor

```sh
ssh root@BOARD_IP 'tail -f /root/luckfox-livekit.log'
```

Expected order:

```text
WAKE READY: native keyword detector loaded; listening for "alexa"
WAKE WORD HEARD: "alexa"
WAKE INTERRUPT: old AI playback stopped and buffered audio discarded
WAKE ACK: LED off; beep wait complete
WAKE WORD: beep complete; MIC1 is capturing one question for the cloud timer.
```

Say “Alexa,” then ask for a story after the beep.
During the story, say “Alexa” again. After the next beep, ask for the time.
Also try a wake without a question. After 1.5 seconds, the cloud must return to standby.

The Go tests cover model configuration, continuous input, microphone routing, playback interruption, and GeckoLink requests.
Compile the tests with the same ARM environment as the client:

```sh
go test -c -mod=vendor -tags nolibopusfile,nosoxr -o luckfox-client.test .
```

On the board, run the test binary. For the native streaming check, provide a raw 16 kHz mono PCM recording:

```sh
WAKE_TEST_BINARY=/opt/luckfox-microwakeword/run-microwakeword.sh \
WAKE_TEST_AUDIO=/root/alexa-test.raw ./luckfox-client.test -test.v
```

The test must detect Alexa before the input pipe closes.
Do not start another recorder while the voice client owns MIC1.

## Measurements and limits

On this RV1106 board, three supplied Alexa recordings produced three detections.
A separate 24.77-second Edison recording produced no Alexa detections.
The detector used approximately 5.2 MiB of RAM.
Offline processing used approximately 1.8% of one core's real-time audio budget.
A live 20-second sample used approximately 1% CPU, including pauses during question capture.
These figures exclude the Go client, GeckoLink, and other board services.

The user confirmed live wake-up, question answers, and interruption.
Three test clips do not establish general accuracy or a false-wake rate.
Distance, background noise, and speaker echo still affect detection.
There is no acoustic echo cancellation in this adapter. Playback that says “Alexa” can activate it.
Edison requires its own trained model. The Alexa model cannot recognize Edison by renaming it.

## Recovery

Use the exact archive directory printed by the installer.
Stop the voice service before restoring its saved client and environment file.
For an older PocketSphinx installation, move the archived `luckfox-wakeword` directory back to `/opt/luckfox-wakeword`.
Then restart the voice service.
Keep the archive private because its environment file contains credentials.

Upstream projects: [microWakeWord runtime](https://github.com/OHF-Voice/pymicro-wakeword),
[microfrontend](https://github.com/rhasspy/pymicro-features).
The runtime bundle includes upstream license notices.

## Files in this change

Updated:

- `.gitignore`
- `README.md`
- `reference-code/agent-server/src/agent.py` (log labels and comments)
- `reference-code/luckfox-client/main.go`
- `reference-code/luckfox-client/main_test.go`
- `reference-code/luckfox-client/build/luckfox-livekit-client`
- `reference-code/luckfox-client/deploy/luckfox-livekit.env.example`

Added:

- `docs/MICROWAKEWORD.md`
- `scripts/build_luckfox_microwakeword.sh`
- `reference-code/luckfox-client/wakeword/microwakeword_stream.cpp`
- `reference-code/luckfox-client/deploy/run-microwakeword.sh`
- `reference-code/luckfox-client/deploy/install-microwakeword.sh`

Removed from the active project:

- `scripts/build_luckfox_wakeword.sh`
- `reference-code/luckfox-client/wakeword/stream_keyword.c`
- `reference-code/luckfox-client/build/luckfox-wakeword`

The uncommitted Porcupine prototype source, launcher, build script, and tests were also removed.
The generated Porcupine bundle and previous PocketSphinx binary were archived outside the repository.
Private credentials and original calibration recordings were preserved.
