# Complete Project Workflow

## Goal

The Luckfox Pico Pi A W captures a spoken English or Kannada question and
plays an AI-generated voice answer. The production Python worker runs in
LiveKit Cloud, so a PC is not required during normal use.

## Deployed architecture

```text
User
  -> Luckfox microphone
  -> Go client encodes and publishes audio
  -> LiveKit Cloud room
  -> Python agent named my-agent
  -> Sarvam Saaras v4 speech-to-text
  -> wake-word and turn-handling logic
  -> DeepSeek V4 Flash
  -> Sarvam Bulbul v3 text-to-speech
  -> LiveKit Cloud room
  -> Luckfox Go client
  -> speaker/headphones
```

English uses Bulbul `en-IN`; Kannada uses Bulbul `kn-IN`. Saaras runs with
automatic language recognition. Deepgram remains only as an English fallback
when Sarvam is not configured.

## Responsibilities

| Component | Responsibility |
|---|---|
| Luckfox Pico Pi A W | Captures microphone PCM, publishes Opus audio, receives reply audio, and plays it through ALSA. |
| LiveKit Cloud | Creates the real-time room, transports audio, dispatches the worker, and hosts the deployed Python agent. |
| Python script | Controls wake-word filtering, language-specific TTS, turn handling, tools, transcripts, and the AI pipeline. |
| Sarvam Saaras v4 | Converts English or Kannada speech into text. |
| DeepSeek V4 Flash | Understands the question, selects tools, and generates the answer. |
| Sarvam Bulbul v3 | Converts the answer to English or Kannada speech. |

## Audio sequence

1. `/etc/init.d/S99zz-livekit-voice` starts at board boot.
2. `/root/luckfox-livekit-client` creates a `luckfox-voice-<timestamp>` room.
3. The client dispatches `my-agent` and publishes the microphone track.
4. Sarvam Saaras produces a final transcript.
5. The Python script accepts the turn only when it contains the required wake
   phrase.
6. DeepSeek generates an answer or calls the calculator, time, weather, or
   project-information tool.
7. Sarvam Bulbul streams synthesized speech back through LiveKit.
8. The board receives and plays the audio through its configured ALSA device.

## Wake word and interruption

Every independent question must start with:

```text
Hey Siri, ...
```

Speech without the phrase is logged and ignored. Saying only `Hey Siri`
opens a ten-second window and produces `Yes, I'm listening.`

Automatic interruption is disabled because the nearby speaker can be
captured by the microphone and mistaken for a user. Instead, explicit
wake-word barge-in is implemented:

```text
Assistant is speaking
User: Hey Siri, what is the time?
Old reply stops
New question is processed
```

LiveKit VAD and turn detection identify speech boundaries. The project’s
Python code adds the wake-word gate, echo rejection, duplicate-turn
suppression, and explicit barge-in behavior.

## Tools

- Calculator: deterministic arithmetic
- Current time/date: timezone-aware live result
- Weather: current Open-Meteo data
- Project database: stored architecture information

## Runtime requirements

Normal autonomous operation requires:

- board power;
- microphone and speaker/headphones;
- configured 2.4 GHz Wi-Fi and internet;
- valid LiveKit, Sarvam, and DeepSeek services;
- a running LiveKit Cloud deployment.

ADB and SSH are maintenance interfaces only. Disconnecting ADB does not stop
the voice agent. Removing Ethernet is also safe after Wi-Fi has connected and
has a working default route and DNS server.

## Current status

- Working: board audio, LiveKit transport, cloud worker, English/Kannada,
  Sarvam STT/TTS, DeepSeek, tools, wake word, explicit barge-in, automatic
  board startup, and cloud/board logs.
- Experimental/pending: Bluetooth Wi-Fi provisioning and GPIO push-to-talk.

The planned GPIO design is documented in
[07-GPIO-PUSH-TO-TALK.md](07-GPIO-PUSH-TO-TALK.md).
