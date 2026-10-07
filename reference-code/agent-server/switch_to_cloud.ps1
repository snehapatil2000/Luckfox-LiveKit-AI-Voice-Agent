param(
    [Parameter(Mandatory = $true)]
    [string]$BoardIP
)

$ErrorActionPreference = "Stop"

Write-Host "Switching Luckfox $BoardIP to the LiveKit Cloud agent..."
Write-Host "Enter the Luckfox SSH password when requested."
ssh "root@$BoardIP" "/root/use-cloud-agent.sh"
if ($LASTEXITCODE -ne 0) {
    throw "Could not switch the Luckfox to cloud mode."
}

Write-Host "Cloud mode is active. This PC can now be switched off."
