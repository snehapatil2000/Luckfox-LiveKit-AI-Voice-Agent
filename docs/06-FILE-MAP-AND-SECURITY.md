# File Map and Security

## Package contents

```text
Luckfox-LiveKit-AI-Voice-Agent-With-PC/
├── README.md
├── START-WITH-PC-DEMO.ps1
├── DEPLOY-TO-LUCKFOX.ps1
├── docs/
│   ├── 01-FULL-PROJECT-WORKFLOW.md
│   ├── 02-BUILD-AND-DEPLOY.md
│   ├── 03-RUN-TEST-AND-LOGS.md
│   ├── 04-TROUBLESHOOTING.md
│   ├── 05-INSTRUCTOR-DEMO.md
│   └── 06-FILE-MAP-AND-SECURITY.md
├── scripts/
│   ├── build_luckfox_client.sh
│   ├── deploy_luckfox_with_pc.ps1
│   ├── restart_luckfox_voice.ps1
│   ├── run_with_pc_demo.ps1
│   └── show_luckfox_logs.ps1
└── reference-code/
    ├── agent-server/
    │   ├── .env.example
    │   ├── pyproject.toml
    │   ├── uv.lock
    │   └── src/agent.py
    └── luckfox-client/
        ├── main.go
        ├── go.mod
        ├── go.sum
        ├── livekit-armv7.patch
        └── deploy/
```

`START-WITH-PC-DEMO.ps1` is the main instructor demonstration command. `DEPLOY-TO-LUCKFOX.ps1` is needed only for a fresh board or a client reinstall.


## Working files on Windows

```text
C:\Users\sneha\Downloads\agent-server\src\agent.py
C:\Users\sneha\Downloads\agent-server\.env.local
C:\Users\sneha\Downloads\agent-server\pyproject.toml
```

## Installed files on Luckfox

```text
/root/luckfox-livekit-client
/root/luckfox-livekit.env
/root/run-luckfox-livekit.sh
/root/use-pc-agent.sh
/etc/init.d/S99zz-livekit-voice
/root/luckfox-livekit.log
```

## Credential placement

| Secret | PC `.env.local` | Luckfox prototype | Documentation/Git |
|---|---:|---:|---:|
| LiveKit URL | Yes | Yes | URL may be documented |
| LiveKit API key | Yes | Yes | Never |
| LiveKit API secret | Yes | Yes | Never |
| DeepSeek key | Yes | No | Never |
| Deepgram key | Yes | No | Never |

The board stores LiveKit API credentials only because this prototype creates its own room token and dispatch. Protect the file:

```sh
chmod 600 /root/luckfox-livekit.env
```

For production, do not permanently store the LiveKit API secret on an embedded device. Use a trusted HTTPS token server that gives the board a short-lived participant token.

## Before committing to Git

Verify that secrets are not staged:

```powershell
git status
git diff --cached
```

Recommended `.gitignore` entries:

```gitignore
.env
.env.*
!.env.example
*.key
*.pem
luckfox-livekit.env
```
