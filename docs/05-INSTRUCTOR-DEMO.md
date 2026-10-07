# Instructor Demonstration

## Introduction

> “This is my bilingual AI voice agent built with a Luckfox Pico Pi A W. The
> board captures my voice and plays the response. LiveKit Cloud transports the
> live audio and runs my Python script.”

## Architecture explanation

> “First, the Luckfox microphone sends audio through LiveKit Cloud. Sarvam
> Saaras converts English or Kannada speech into text. DeepSeek understands
> the question and generates the answer. Sarvam Bulbul converts the answer
> into English or Kannada voice. LiveKit sends that voice back to the Luckfox
> speaker or headphones.”

## Start the demo

Restart the board service if necessary:

```powershell
adb -s <board-serial> shell /etc/init.d/S99zz-livekit-voice restart
```

Open LiveKit Cloud logs in another terminal:

```powershell
lk agent logs --id <cloud-agent-id> --project <project-name> --log-type deploy
```

Wait until the board log says `Microphone is live` and cloud logs show
`VOICE STATE: listening`.

## Demonstration questions

```text
Hey Siri, what time is it?
Hey Siri, tell me the districts of Karnataka.
Hey Siri, ಕರ್ನಾಟಕದ ಜಿಲ್ಲೆಗಳನ್ನು ಹೇಳು.
```

For barge-in, ask for a story and interrupt it with:

```text
Hey Siri, what is the weather in Bengaluru?
```

Say:

> “This feature is called wake-word barge-in or interruption handling. LiveKit
> detects speech, but the Python script interrupts only when the final
> transcript contains Hey Siri. This prevents speaker echo from stopping the
> answer.”

## Common questions and answers

**What does LiveKit do?**

> “LiveKit provides the real-time room, transports audio in both directions,
> dispatches the worker, and hosts my deployed Python agent. It is not the
> language model.”

**What is the Python voice agent?**

> “It is the script that connects STT, DeepSeek, TTS, wake-word logic, tools,
> turn detection, interruption handling, and logs.”

**Does the PC have to remain connected?**

> “No. The normal agent runs in LiveKit Cloud. The board needs power, audio,
> Wi-Fi, and internet. I use the PC only for development and logs.”

**What are STT, LLM, and TTS?**

> “STT converts speech to text, the LLM generates the answer, and TTS converts
> the answer back into speech.”

**What logs are shown?**

> “The transcript logs come from the Python agent in LiveKit Cloud. Board logs
> show audio tracks and the network connection but not the complete question
> and answer text.”

**What remains unfinished?**

> “Bluetooth Wi-Fi provisioning and GPIO push-to-talk are experimental. The
> demonstrated voice path and automatic board startup are working.”

## Services summary

```text
Hardware audio: Luckfox Pico Pi A W
Real-time transport and agent hosting: LiveKit Cloud
Speech-to-text: Sarvam Saaras v4
Language model: DeepSeek V4 Flash
Text-to-speech: Sarvam Bulbul v3
Wake phrase: Hey Siri
Interruption: explicit wake-word barge-in
```
