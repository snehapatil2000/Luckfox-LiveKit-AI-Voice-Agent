param(
    [Parameter(Mandatory = $true)]
    [string]$BoardIP,
    [string]$BinaryPath = "",
    [switch]$InstallPhoneApp
)

$ErrorActionPreference = "Stop"
$ProjectRoot = $PSScriptRoot
$ClientRoot = Join-Path $ProjectRoot "reference-code\luckfox-client"
$ProvisionRoot = Join-Path $ProjectRoot "luckfox-wifi-provision"

if ([string]::IsNullOrWhiteSpace($BinaryPath)) {
    $BinaryPath = Join-Path $ClientRoot "build\luckfox-livekit-client"
}

$VoiceFiles = @{
    $BinaryPath = "/tmp/luckfox-livekit-client.new"
    $(Join-Path $ClientRoot "deploy\run-luckfox-livekit.sh") = "/tmp/run-luckfox-livekit.sh.new"
    $(Join-Path $ClientRoot "deploy\use-pc-agent.sh") = "/tmp/use-pc-agent.sh.new"
    $(Join-Path $ClientRoot "deploy\S99zz-livekit-voice") = "/tmp/S99zz-livekit-voice.new"
    $(Join-Path $ClientRoot "deploy\luckfox-livekit.env.example") = "/tmp/luckfox-livekit.env.example.new"
}

$BluetoothFiles = @{
    $(Join-Path $ProvisionRoot "S98wifi-provision") = "/tmp/S98wifi-provision.new"
    $(Join-Path $ProvisionRoot "provision-supervisor.sh") = "/tmp/provision-supervisor.sh.new"
    $(Join-Path $ProvisionRoot "handle-connection.sh") = "/tmp/handle-connection.sh.new"
    $(Join-Path $ProvisionRoot "provision.py") = "/tmp/provision.py.new"
    $(Join-Path $ProvisionRoot "pin") = "/tmp/luckfox-wifi-pin.new"
}

foreach ($Item in @($VoiceFiles.GetEnumerator()) + @($BluetoothFiles.GetEnumerator())) {
    if (-not (Test-Path $Item.Key)) {
        throw "Required deployment file is missing: $($Item.Key)"
    }
}

Write-Host "Deploying complete Luckfox AI Voice Agent bundle to $BoardIP" -ForegroundColor Cyan
Write-Host "This installs LiveKit voice plus Bluetooth Wi-Fi provisioning." -ForegroundColor Cyan

foreach ($Item in @($VoiceFiles.GetEnumerator()) + @($BluetoothFiles.GetEnumerator())) {
    scp $Item.Key "root@${BoardIP}:$($Item.Value)"
    if ($LASTEXITCODE -ne 0) { throw "Upload failed: $($Item.Key)" }
}

$InstallCommand = @(
    "/etc/init.d/S99zz-livekit-voice stop 2>/dev/null || true",
    "/etc/init.d/S98wifi-provision stop 2>/dev/null || true",
    "mv /tmp/luckfox-livekit-client.new /root/luckfox-livekit-client",
    "mv /tmp/run-luckfox-livekit.sh.new /root/run-luckfox-livekit.sh",
    "mv /tmp/use-pc-agent.sh.new /root/use-pc-agent.sh",
    "mv /tmp/S99zz-livekit-voice.new /etc/init.d/S99zz-livekit-voice",
    "mv /tmp/luckfox-livekit.env.example.new /root/luckfox-livekit.env.example",
    "mkdir -p /opt/luckfox-wifi-provision",
    "mv /tmp/S98wifi-provision.new /etc/init.d/S98wifi-provision",
    "mv /tmp/provision-supervisor.sh.new /opt/luckfox-wifi-provision/provision-supervisor.sh",
    "mv /tmp/handle-connection.sh.new /opt/luckfox-wifi-provision/handle-connection.sh",
    "mv /tmp/provision.py.new /opt/luckfox-wifi-provision/provision.py",
    "mv /tmp/luckfox-wifi-pin.new /opt/luckfox-wifi-provision/pin",
    "chmod 755 /root/luckfox-livekit-client /root/run-luckfox-livekit.sh /root/use-pc-agent.sh /etc/init.d/S99zz-livekit-voice /etc/init.d/S98wifi-provision /opt/luckfox-wifi-provision/*.sh /opt/luckfox-wifi-provision/provision.py",
    "chmod 600 /opt/luckfox-wifi-provision/pin",
    "if [ -d /etc/rc.d ]; then ln -sf /etc/init.d/S99zz-livekit-voice /etc/rc.d/S99zz-livekit-voice; ln -sf /etc/init.d/S98wifi-provision /etc/rc.d/S98wifi-provision; fi",
    "if [ -f /root/luckfox-livekit.env ]; then chmod 600 /root/luckfox-livekit.env; else cp /root/luckfox-livekit.env.example /root/luckfox-livekit.env; chmod 600 /root/luckfox-livekit.env; echo CONFIG_REQUIRED:/root/luckfox-livekit.env; fi",
    "/etc/init.d/S98wifi-provision start",
    "/etc/init.d/S99zz-livekit-voice start"
) -join "; "

ssh "root@$BoardIP" $InstallCommand
if ($LASTEXITCODE -ne 0) { throw "Remote installation failed." }

if ($InstallPhoneApp) {
    $Apk = Join-Path $ProjectRoot "luckfox-wifi-provision-app\app\build\outputs\apk\debug\app-debug.apk"
    if (-not (Test-Path $Apk)) {
        throw "Phone APK not found. Build it first with gradlew.bat assembleDebug."
    }
    adb install -r $Apk
    if ($LASTEXITCODE -ne 0) { throw "Phone APK installation failed." }
}

Write-Host "Complete deployment finished." -ForegroundColor Green
Write-Host "Board voice service: /etc/init.d/S99zz-livekit-voice" -ForegroundColor Green
Write-Host "Board Bluetooth Wi-Fi service: /etc/init.d/S98wifi-provision" -ForegroundColor Green
Write-Host "The PC is not required after Wi-Fi and /root/luckfox-livekit.env are configured." -ForegroundColor Green
