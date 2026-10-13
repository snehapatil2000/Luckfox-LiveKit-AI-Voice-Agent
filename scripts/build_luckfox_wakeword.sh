#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd -- "$(dirname -- "$0")" && pwd)"
client_dir="$script_dir/../reference-code/luckfox-client"
wake_prefix="${POCKETSPHINX_PREFIX:-/tmp/pocketsphinx-arm}"
# For the RV1106, link a Release PocketSphinx library built with
# -mcpu=cortex-a7 -mfpu=neon-vfpv4 -mfloat-abi=hard -ffast-math.
# POCKETSPHINX_LIBRARY selects that archive without replacing model files.
wake_library="${POCKETSPHINX_LIBRARY:-$wake_prefix/lib/libpocketsphinx.a}"
cross_cc="${WAKE_CC:-/home/sneha/luckfox-pico-sdk/tools/linux/toolchain/arm-rockchip830-linux-uclibcgnueabihf/bin/arm-rockchip830-linux-uclibcgnueabihf-gcc}"

[[ -f "$wake_library" ]] || {
    echo "Set POCKETSPHINX_PREFIX to the ARM PocketSphinx installation." >&2
    exit 1
}
mkdir -p "$client_dir/build"
"$cross_cc" -O3 -Wall -Wextra -std=c99 \
    -I"$wake_prefix/include" -I"$wake_prefix/include/pocketsphinx" \
    "$client_dir/wakeword/stream_keyword.c" \
    "$wake_library" -lm \
    -o "$client_dir/build/luckfox-wakeword"
file "$client_dir/build/luckfox-wakeword"
