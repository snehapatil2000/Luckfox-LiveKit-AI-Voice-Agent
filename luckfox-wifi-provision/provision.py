#!/usr/bin/env python3
"""Receive Wi-Fi credentials over an RFCOMM TTY and apply them safely."""

from __future__ import annotations

import json
import os
import re
import shutil
import subprocess
import sys
import tempfile
import time
from datetime import datetime
from pathlib import Path


WPA_CONFIG = Path("/etc/wpa_supplicant.conf")
BACKUP_DIR = Path("/var/lib/luckfox-wifi-provision/backups")
LOG_FILE = Path("/var/log/luckfox-wifi-provision.log")
WIFI_INTERFACE = "wlan0"
MAX_INPUT = 512


def log(message: str) -> None:
    timestamp = datetime.now().astimezone().isoformat(timespec="seconds")
    safe_message = message.replace("\n", " ").replace("\r", " ")
    with LOG_FILE.open("a", encoding="utf-8") as log_handle:
        log_handle.write(f"{timestamp} {safe_message}\n")


def send(serial, message: str) -> None:
    serial.write((message + "\r\n").encode("utf-8"))
    serial.flush()


def read_line(serial) -> str:
    raw = serial.readline(MAX_INPUT + 1)
    if len(raw) > MAX_INPUT:
        raise ValueError("Input is too long")
    if not raw:
        raise ConnectionError("Phone disconnected")
    return raw.decode("utf-8", errors="strict").strip("\r\n")


def validate_credentials(ssid: str, password: str) -> None:
    if not ssid or len(ssid.encode("utf-8")) > 32:
        raise ValueError("Wi-Fi name must contain 1 to 32 bytes")
    if any(char in ssid for char in "\x00\r\n"):
        raise ValueError("Wi-Fi name contains an invalid character")
    is_raw_psk = bool(re.fullmatch(r"[0-9A-Fa-f]{64}", password))
    if not is_raw_psk and not 8 <= len(password) <= 63:
        raise ValueError("Wi-Fi password must contain 8 to 63 characters")
    if any(char in password for char in "\x00\r\n"):
        raise ValueError("Wi-Fi password contains an invalid character")


def quote_wpa(value: str) -> str:
    return '"' + value.replace("\\", "\\\\").replace('"', '\\"') + '"'


def make_config(ssid: str, password: str) -> str:
    psk_value = password if re.fullmatch(r"[0-9A-Fa-f]{64}", password) else quote_wpa(password)
    return (
        "ctrl_interface=/var/run/wpa_supplicant\n"
        "ap_scan=1\n"
        "update_config=1\n\n"
        "network={\n"
        f"        ssid={quote_wpa(ssid)}\n"
        f"        psk={psk_value}\n"
        "        key_mgmt=WPA-PSK\n"
        "}\n"
    )


def run(command: list[str], timeout: int = 20) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        command,
        stdin=subprocess.DEVNULL,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        text=True,
        timeout=timeout,
        check=False,
    )


def reconnect_wifi() -> tuple[bool, str]:
    run(["/usr/bin/wpa_cli", "-i", WIFI_INTERFACE, "terminate"], timeout=5)
    time.sleep(1)
    run(["/sbin/ip", "addr", "flush", "dev", WIFI_INTERFACE], timeout=5)
    run(["/sbin/ip", "link", "set", WIFI_INTERFACE, "up"], timeout=5)

    result = run(
        [
            "/usr/bin/wpa_supplicant",
            "-B",
            "-i",
            WIFI_INTERFACE,
            "-c",
            str(WPA_CONFIG),
        ],
        timeout=10,
    )
    if result.returncode != 0:
        return False, "wpa_supplicant could not start"

    result = run(
        ["/sbin/udhcpc", "-i", WIFI_INTERFACE, "-n", "-q", "-t", "10", "-T", "2"],
        timeout=30,
    )
    if result.returncode != 0:
        return False, "Wi-Fi connected but did not receive an IP address"

    address_result = run(["/sbin/ip", "-4", "addr", "show", "dev", WIFI_INTERFACE], timeout=5)
    match = re.search(r"\binet\s+(\d+\.\d+\.\d+\.\d+)/", address_result.stdout)
    if not match:
        return False, "No Wi-Fi IP address was assigned"

    ping_result = run(["/bin/ping", "-I", WIFI_INTERFACE, "-c", "1", "-W", "4", "1.1.1.1"], timeout=8)
    if ping_result.returncode != 0:
        return False, "Wi-Fi has an IP address but cannot reach the internet"

    return True, match.group(1)


def replace_config(config_text: str) -> None:
    WPA_CONFIG.parent.mkdir(parents=True, exist_ok=True)
    descriptor, temporary_name = tempfile.mkstemp(prefix="wpa-", dir=str(WPA_CONFIG.parent))
    temporary_path = Path(temporary_name)
    try:
        with os.fdopen(descriptor, "w", encoding="utf-8") as handle:
            handle.write(config_text)
            handle.flush()
            os.fsync(handle.fileno())
        os.chmod(temporary_path, 0o600)
        os.replace(temporary_path, WPA_CONFIG)
    finally:
        temporary_path.unlink(missing_ok=True)


def restart_voice_agent() -> None:
    startup_script = Path("/etc/init.d/S99zz-livekit-voice")
    if startup_script.exists():
        subprocess.Popen(
            ["/bin/sh", "-c", "sleep 3; /etc/init.d/S99zz-livekit-voice restart"],
            stdin=subprocess.DEVNULL,
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
            start_new_session=True,
        )


def apply_credentials(ssid: str, password: str) -> tuple[bool, str]:
    validate_credentials(ssid, password)
    BACKUP_DIR.mkdir(parents=True, exist_ok=True)
    os.chmod(BACKUP_DIR.parent, 0o700)
    os.chmod(BACKUP_DIR, 0o700)

    backup_path: Path | None = None
    if WPA_CONFIG.exists():
        stamp = datetime.now().strftime("%Y%m%d-%H%M%S")
        backup_path = BACKUP_DIR / f"wpa_supplicant.conf.{stamp}.bak"
        shutil.copy2(WPA_CONFIG, backup_path)
        os.chmod(backup_path, 0o600)

    replace_config(make_config(ssid, password))
    success, detail = reconnect_wifi()
    if success:
        log(f"Wi-Fi provisioning succeeded for SSID={ssid!r}, IP={detail}")
        restart_voice_agent()
        return True, detail

    log(f"Wi-Fi provisioning failed for SSID={ssid!r}: {detail}; restoring backup")
    if backup_path is not None:
        shutil.copy2(backup_path, WPA_CONFIG)
        os.chmod(WPA_CONFIG, 0o600)
        reconnect_wifi()
        restart_voice_agent()
    return False, detail


def receive_credentials(serial) -> tuple[str, str]:
    send(serial, "Luckfox Wi-Fi setup")
    send(serial, "Enter Wi-Fi name (SSID), then press Send:")
    first_line = read_line(serial)

    if first_line.lstrip().startswith("{"):
        payload = json.loads(first_line)
        return str(payload["ssid"]), str(payload["password"])

    send(serial, "Enter Wi-Fi password, then press Send:")
    password = read_line(serial)
    return first_line, password


def main() -> int:
    tty_path = Path(sys.argv[1] if len(sys.argv) > 1 else "/dev/rfcomm0")
    log(f"Phone connected on {tty_path}")
    try:
        with tty_path.open("r+b", buffering=0) as serial:
            ssid, password = receive_credentials(serial)
            send(serial, "Applying and testing Wi-Fi. Please wait up to 45 seconds...")
            success, detail = apply_credentials(ssid, password)
            if success:
                send(serial, f"SUCCESS: Wi-Fi connected. Luckfox IP is {detail}")
                send(serial, "LiveKit voice agent is restarting on Wi-Fi.")
                return 0
            send(serial, f"FAILED: {detail}")
            send(serial, "The previous Wi-Fi configuration was restored.")
            return 1
    except (ConnectionError, UnicodeError, ValueError, KeyError, json.JSONDecodeError) as error:
        log(f"Provisioning input error: {type(error).__name__}: {error}")
        return 2
    except Exception as error:  # Keep the RFCOMM watcher alive after unexpected failures.
        log(f"Unexpected provisioning error: {type(error).__name__}: {error}")
        return 3


if __name__ == "__main__":
    raise SystemExit(main())
