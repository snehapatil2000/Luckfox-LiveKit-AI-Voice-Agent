# File Map and Security

## Important files

```text
README.md
DEPLOY-FRESH-LUCKFOX.ps1
docs/
reference-code/agent-server/
├── .env.example
├── Dockerfile
├── pyproject.toml
├── uv.lock
├── src/agent.py
└── tests/
reference-code/luckfox-client/
├── main.go
├── build/luckfox-livekit-client
└── deploy/
    ├── S99zz-livekit-voice
    ├── run-luckfox-livekit.sh
    └── luckfox-livekit.env.example
luckfox-wifi-provision/             experimental
luckfox-wifi-provision-app/         experimental
docs/07-GPIO-PUSH-TO-TALK.md        planned hardware trigger
```

## Installed board files

```text
/root/luckfox-livekit-client
/root/luckfox-livekit.env
/root/run-luckfox-livekit.sh
/etc/init.d/S99zz-livekit-voice
/root/luckfox-livekit.log
```

## Secret placement

| Value | Local `.env.local` | LiveKit deployment secret | Board | Git |
|---|---:|---:|---:|---:|
| LiveKit URL | Yes | Yes | Yes | Placeholder only |
| LiveKit API key/secret | Yes | Yes | Prototype only | Never |
| DeepSeek key | Yes | Yes | No | Never |
| Sarvam key | Yes | Yes | No | Never |
| Deepgram fallback key | Optional | Optional | No | Never |

Protect the board environment:

```sh
chmod 600 /root/luckfox-livekit.env
```

Production hardware should not permanently hold a LiveKit API secret. A
trusted token service should issue short-lived participant tokens.

## Before every commit

```bash
git status --short
git diff --cached
git ls-files | rg -i '(^|/)(\.env($|\.)|.*secret.*|.*\.pem$|.*\.key$)'
```

Tracked `.env.example` files must contain only empty values or obvious
placeholders. Never paste credentials into documentation, screenshots, logs,
issues, or commits.

The repository includes a prebuilt ARM binary and vendored Go dependencies so
the embedded client can be reproduced in the tested environment. Generated
Android build output, local virtual environments, logs, and credentials are
ignored.
