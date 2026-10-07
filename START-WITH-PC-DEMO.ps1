param(
    [string]$BoardIP = "192.168.0.106",
    [string]$AgentDirectory = "$env:USERPROFILE\Downloads\agent-server"
)

$ErrorActionPreference = "Stop"
$Script = Join-Path $PSScriptRoot "scripts\run_with_pc_demo.ps1"
& $Script -BoardIP $BoardIP -AgentDirectory $AgentDirectory

