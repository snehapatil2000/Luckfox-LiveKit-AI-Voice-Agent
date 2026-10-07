param(
    [Parameter(Mandatory = $true)]
    [string]$BoardIP
)

$ErrorActionPreference = "Stop"
$helperDirectory = Join-Path $PSScriptRoot "luckfox-deploy"
$pcHelper = Join-Path $helperDirectory "use-pc-agent.sh"
$cloudHelper = Join-Path $helperDirectory "use-cloud-agent.sh"

if (-not (Test-Path $pcHelper) -or -not (Test-Path $cloudHelper)) {
    throw "The luckfox-deploy helper files are missing."
}

Write-Host "Copying mode-switch helpers to Luckfox $BoardIP..."
Write-Host "Enter the Luckfox SSH password when requested."
scp $pcHelper $cloudHelper "root@${BoardIP}:/root/"
if ($LASTEXITCODE -ne 0) {
    throw "Could not copy helper scripts to the Luckfox."
}

ssh "root@$BoardIP" "chmod 755 /root/use-pc-agent.sh /root/use-cloud-agent.sh; /root/use-cloud-agent.sh"
if ($LASTEXITCODE -ne 0) {
    throw "Could not activate the helper scripts on the Luckfox."
}

Write-Host "Mode-switch helpers installed. Luckfox is in cloud mode."
