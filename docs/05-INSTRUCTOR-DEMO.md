# Simple Instructor Demonstration

## Start the demonstration

Show the Luckfox and say:

> “This is the Luckfox Pico Pi A W. Its microphone captures my voice, and its speaker or headphones play the AI response.”

Open PowerShell and run:

```powershell
cd "C:\Users\sneha\OneDrive\Documents\PROJECTS\AI VOICE AGENT\Livekit-Deepseek-Deepgram\Luckfox-LiveKit-AI-Voice-Agent-With-PC"
powershell -ExecutionPolicy Bypass -File .\START-WITH-PC-DEMO.ps1
```

Say:

> “This script selects PC mode on the Luckfox and starts my Python LiveKit agent on Windows. The PC must stay on in this mode.”

When `VOICE STATE: listening` appears, say:

> “The Luckfox is now connected to a LiveKit Cloud room and is streaming microphone audio through the internet.”

Ask near the board:

> “Agent, what is the capital of India?”

Point to the logs and say:

> “Deepgram converts my live speech into text. The wake word is Agent, so speech without that word is ignored. DeepSeek is the LLM that understands the question and generates the answer. Deepgram then converts that answer into voice. LiveKit sends the voice back to the Luckfox speaker.”

Ask:

> “Agent, what time is it?”

Say:

> “Time questions are handled by the Python program using India Standard Time, so the LLM does not guess the current time.”

## Short architecture explanation

> “I use LiveKit Cloud as the real-time communication server. My Python AI agent runs on this Windows PC. Deepgram provides speech-to-text and text-to-speech. DeepSeek provides the large language model. The Luckfox handles the microphone and speaker.”

## If asked whether it is local or cloud

> “It is an internet/cloud voice system with the Python agent running locally on my Windows PC. LiveKit, Deepgram, and DeepSeek are cloud services. Therefore, both the PC and internet are required.”

## If asked what LiveKit does

> “LiveKit creates a real-time room and transports the live microphone and reply audio. It is not the LLM. DeepSeek is the LLM.”

## If asked what an LLM is

> “LLM means Large Language Model. It understands the user's text question and generates a suitable text answer. I use DeepSeek V4 Flash.”

## If asked how this differs from the old WAV design

> “The old design recorded a complete WAV file, transferred it, processed it, and transferred a reply file. The current LiveKit design streams audio continuously, so processing can begin sooner and the conversation feels more real time.”

## Services summary

```text
Microphone and speaker: Luckfox
Audio server: LiveKit Cloud
Agent/orchestration: Python on Windows PC
Speech-to-text: Deepgram Nova-3
Answer generation: DeepSeek V4 Flash
Text-to-speech: Deepgram Aura-2
```
