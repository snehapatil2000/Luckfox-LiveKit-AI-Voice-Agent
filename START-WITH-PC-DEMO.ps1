param(
    [Parameter(Mandatory = $true)]
    [string]$BoardIP,
    [string]$AgentDirectory = "$env:USERPROFILE\Downloads\agent-server"
)

$ErrorActionPreference = "Stop"
$Script = Join-Path $PSScriptRoot "scripts\run_with_pc_demo.ps1"
& $Script -BoardIP $BoardIP -AgentDirectory $AgentDirectory
