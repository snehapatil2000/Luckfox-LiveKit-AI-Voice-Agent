# Luckfox Bluetooth Wi-Fi provisioning

This package adds Bluetooth Classic Wi-Fi setup to the Luckfox Pico Pi A W.
It is separate from the LiveKit voice client and does not contain API keys.

## Phone procedure

1. Keep Ethernet connected for the first test.
2. Install an Android Bluetooth serial terminal application.
3. Pair with `Luckfox-WiFi-Setup`.
4. Connect to its Serial Port Profile.
5. Enter the Wi-Fi SSID and press Send/newline.
6. Enter the Wi-Fi password and press Send/newline.
7. Wait for `SUCCESS` and note the Wi-Fi IP address.
8. Disconnect Ethernet and test the LiveKit voice agent.

The board is discoverable for 15 minutes after boot. Run
`/etc/init.d/S98wifi-provision restart` to open another 15-minute window.

Logs are stored at `/var/log/luckfox-wifi-provision.log`. Wi-Fi backups are
stored with mode 0600 in `/var/lib/luckfox-wifi-provision/backups/`.
