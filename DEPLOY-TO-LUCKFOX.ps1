param(
    [Parameter(Mandatory = $true)]
    [string]$BoardIP,
    [string]$BinaryPath = ""
)

$ErrorActionPreference = "Stop"
$Script = Join-Path $PSScriptRoot "scripts\deploy_luckfox_with_pc.ps1"
& $Script -BoardIP $BoardIP -BinaryPath $BinaryPath
