param(
    [string]$BoardIP = "192.168.0.106",
    [int]$Lines = 100
)

$ErrorActionPreference = "Stop"

Write-Host "Following Luckfox logs from $BoardIP. Press Ctrl+C to stop." -ForegroundColor Cyan
Write-Host "Enter the Luckfox SSH password when requested."
ssh "root@$BoardIP" "tail -n $Lines -F /root/luckfox-livekit.log"
if ($LASTEXITCODE -ne 0) {
    throw "Could not read the Luckfox log."
}

