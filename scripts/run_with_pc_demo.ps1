param(
    [Parameter(Mandatory = $true)]
    [string]$BoardIP,
    [string]$AgentDirectory = "$env:USERPROFILE\Downloads\agent-server"
)

$ErrorActionPreference = "Stop"

if (-not (Test-Path (Join-Path $AgentDirectory "src\agent.py"))) {
    throw "Agent code was not found at $AgentDirectory\src\agent.py"
}
if (-not (Test-Path (Join-Path $AgentDirectory ".env.local"))) {
    throw "Missing $AgentDirectory\.env.local. Create it from .env.example and add the five API values."
}
if (-not (Get-Command uv -ErrorAction SilentlyContinue)) {
    throw "uv is not installed or is not available in PATH."
}

Write-Host "Luckfox LiveKit voice demonstration (WITH PC)" -ForegroundColor Cyan
Write-Host "Board: $BoardIP"
Write-Host "Agent folder: $AgentDirectory"
Write-Host "Enter the Luckfox SSH password when requested."

ssh "root@$BoardIP" "/root/use-pc-agent.sh"
if ($LASTEXITCODE -ne 0) {
    throw "Could not switch the Luckfox to my-agent-local."
}

Set-Location $AgentDirectory
$env:LIVEKIT_AGENT_NAME = "my-agent-local"

Write-Host ""
Write-Host "Starting the local Python agent. Keep this window open." -ForegroundColor Green
Write-Host "Wait for VOICE STATE: listening. Then speak near Luckfox."
Write-Host "Example: Hey Siri, what is the capital of India?"
Write-Host "Press Ctrl+C to stop."

uv run src/agent.py dev
