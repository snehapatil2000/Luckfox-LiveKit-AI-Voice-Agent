param(
    [string]$BoardIP = "192.168.0.106",
    [string]$BinaryPath = ""
)

$ErrorActionPreference = "Stop"
$Script = Join-Path $PSScriptRoot "scripts\deploy_luckfox_with_pc.ps1"
& $Script -BoardIP $BoardIP -BinaryPath $BinaryPath

