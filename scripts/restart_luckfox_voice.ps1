param(
    [Parameter(Mandatory = $true)]
    [string]$BoardIP
)

$ErrorActionPreference = "Stop"

Write-Host "Restarting the existing Luckfox voice service on $BoardIP..."
Write-Host "Enter the Luckfox SSH password when requested."
ssh "root@$BoardIP" "/etc/init.d/S99zz-livekit-voice restart"
if ($LASTEXITCODE -ne 0) {
    throw "Luckfox service restart failed."
}

Write-Host "Restart complete. Watch the PC agent for a new job request." -ForegroundColor Green
