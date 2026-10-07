param(
    [string]$BoardIP = "192.168.1.106"
)

$ErrorActionPreference = "Stop"
Set-Location $PSScriptRoot

Write-Host "Switching Luckfox $BoardIP to the local PC agent..."
Write-Host "Enter the Luckfox SSH password when requested."
ssh "root@$BoardIP" "/root/use-pc-agent.sh"
if ($LASTEXITCODE -ne 0) {
    throw "Could not switch the Luckfox to PC mode."
}

$env:LIVEKIT_AGENT_NAME = "my-agent-local"
Write-Host "Starting the LiveKit agent on this PC. Press Ctrl+C to stop."
Write-Host "Voice transcripts will appear as VOICE USER and VOICE ASSISTANT."
uv run src/agent.py dev
