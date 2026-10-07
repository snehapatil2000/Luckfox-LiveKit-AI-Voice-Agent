# Luckfox Wi-Fi Setup application

This is a separate Android application for configuring Wi-Fi on a Luckfox Pico Pi W.
It does not change the LiveKit voice APK.

## How it works

1. The Luckfox advertises itself as `Luckfox-WiFi-Setup` over Bluetooth Classic.
2. The phone pairs with it and the app opens a Bluetooth SPP/RFCOMM connection.
3. The app sends one JSON line containing the Wi-Fi SSID and password.
4. The Luckfox writes `/etc/wpa_supplicant.conf`, connects `wlan0`, obtains an IP,
   tests internet access, and restarts the existing LiveKit service.

## Build the APK

From WSL:

```bash
cd /home/sneha/agent/luckfox-wifi-provision-app
./gradlew assembleDebug
```

APK output:

`app/build/outputs/apk/debug/app-debug.apk`

Install from Windows PowerShell after connecting the phone by USB:

```powershell
adb install -r "\\wsl.localhost\Ubuntu-22.04\home\sneha\agent\luckfox-wifi-provision-app\app\build\outputs\apk\debug\app-debug.apk"
```

## Board requirement

The board must have the existing receiver service installed and running:

```text
/opt/luckfox-wifi-provision/provision.py
/opt/luckfox-wifi-provision/provision-supervisor.sh
/etc/init.d/S98wifi-provision
```

Start it on the board with:

```sh
/etc/init.d/S98wifi-provision start
```

Then open the app, tap **Find Luckfox**, select or enter the board Bluetooth
address, enter the Wi-Fi name and password, and tap **Send Wi-Fi settings**.

The app uses RFCOMM channel 3, matching the board receiver. If Android asks
for pairing, accept the request shown on the phone and retry the send once.

The Wi-Fi password is sent but is not saved by the Android app.
