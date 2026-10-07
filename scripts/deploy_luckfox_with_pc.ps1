param(
    [string]$BoardIP = "192.168.0.106",
    [string]$BinaryPath = ""
)

$ErrorActionPreference = "Stop"
$PackageRoot = Split-Path -Parent $PSScriptRoot
$ClientRoot = Join-Path $PackageRoot "reference-code\luckfox-client"

if ([string]::IsNullOrWhiteSpace($BinaryPath)) {
    $BinaryPath = Join-Path $ClientRoot "build\luckfox-livekit-client"
}

$Runner = Join-Path $ClientRoot "deploy\run-luckfox-livekit.sh"
$PcHelper = Join-Path $ClientRoot "deploy\use-pc-agent.sh"
$InitScript = Join-Path $ClientRoot "deploy\S99zz-livekit-voice"
$EnvTemplate = Join-Path $ClientRoot "deploy\luckfox-livekit.env.example"

foreach ($File in @($BinaryPath, $Runner, $PcHelper, $InitScript, $EnvTemplate)) {
    if (-not (Test-Path $File)) {
        throw "Required deployment file is missing: $File"
    }
}

Write-Host "Deploying the Luckfox LiveKit client to $BoardIP..." -ForegroundColor Cyan
Write-Host "Enter the Luckfox SSH password when requested."

scp $BinaryPath "root@${BoardIP}:/tmp/luckfox-livekit-client.new"
if ($LASTEXITCODE -ne 0) { throw "Binary upload failed." }
scp $Runner "root@${BoardIP}:/tmp/run-luckfox-livekit.sh.new"
if ($LASTEXITCODE -ne 0) { throw "Runner upload failed." }
scp $PcHelper "root@${BoardIP}:/tmp/use-pc-agent.sh.new"
if ($LASTEXITCODE -ne 0) { throw "PC helper upload failed." }
scp $InitScript "root@${BoardIP}:/tmp/S99zz-livekit-voice.new"
if ($LASTEXITCODE -ne 0) { throw "Init script upload failed." }
scp $EnvTemplate "root@${BoardIP}:/tmp/luckfox-livekit.env.example.new"
if ($LASTEXITCODE -ne 0) { throw "Environment template upload failed." }

$InstallCommand = @(
    "/etc/init.d/S99zz-livekit-voice stop 2>/dev/null || true",
    "mv /tmp/luckfox-livekit-client.new /root/luckfox-livekit-client",
    "mv /tmp/run-luckfox-livekit.sh.new /root/run-luckfox-livekit.sh",
    "mv /tmp/use-pc-agent.sh.new /root/use-pc-agent.sh",
    "mv /tmp/S99zz-livekit-voice.new /etc/init.d/S99zz-livekit-voice",
    "mv /tmp/luckfox-livekit.env.example.new /root/luckfox-livekit.env.example",
    "chmod 755 /root/luckfox-livekit-client /root/run-luckfox-livekit.sh /root/use-pc-agent.sh /etc/init.d/S99zz-livekit-voice",
    "if [ -f /root/luckfox-livekit.env ]; then chmod 600 /root/luckfox-livekit.env; /root/use-pc-agent.sh; else echo 'CONFIG_REQUIRED: create /root/luckfox-livekit.env from /root/luckfox-livekit.env.example'; fi"
) -join "; "

ssh "root@$BoardIP" $InstallCommand
if ($LASTEXITCODE -ne 0) {
    throw "Remote installation failed."
}

Write-Host "Deployment completed." -ForegroundColor Green
Write-Host "If CONFIG_REQUIRED appeared, configure /root/luckfox-livekit.env and restart the service."

