#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
PACKAGE_ROOT="$(cd -- "$SCRIPT_DIR/.." && pwd)"
PROJECT_DIR="${PROJECT_DIR:-$PACKAGE_ROOT/reference-code/luckfox-client}"
GO_BIN="${GO_BIN:-/home/sneha/agent/toolchains/go/bin/go}"
CROSS_PREFIX="${CROSS_PREFIX:-/home/sneha/luckfox-pico-sdk/tools/linux/toolchain/arm-rockchip830-linux-uclibcgnueabihf/bin/arm-rockchip830-linux-uclibcgnueabihf-}"
OPUS_VERSION="1.5.2"
OPUS_PREFIX="$PROJECT_DIR/opus-armv7"

for command_name in curl tar make patch pkg-config; do
    command -v "$command_name" >/dev/null || {
        echo "ERROR: required command is missing: $command_name" >&2
        exit 1
    }
done

[[ -x "$GO_BIN" ]] || {
    echo "ERROR: Go was not found at $GO_BIN" >&2
    exit 1
}
[[ -x "${CROSS_PREFIX}gcc" ]] || {
    echo "ERROR: Luckfox cross compiler was not found at ${CROSS_PREFIX}gcc" >&2
    exit 1
}
[[ -f "$PROJECT_DIR/go.mod" ]] || {
    echo "ERROR: Luckfox client source was not found at $PROJECT_DIR" >&2
    exit 1
}

if [[ ! -f "$OPUS_PREFIX/lib/libopus.a" ]]; then
    echo "Building Opus $OPUS_VERSION for Luckfox ARMv7..."
    mkdir -p "$PROJECT_DIR/third_party"
    cd "$PROJECT_DIR/third_party"
    if [[ ! -f "opus-$OPUS_VERSION.tar.gz" ]]; then
        curl -fLO "https://downloads.xiph.org/releases/opus/opus-$OPUS_VERSION.tar.gz"
    fi
    if [[ ! -d "opus-$OPUS_VERSION" ]]; then
        tar -xzf "opus-$OPUS_VERSION.tar.gz"
    fi
    cd "opus-$OPUS_VERSION"
    ./configure \
        --host=arm-rockchip830-linux-uclibcgnueabihf \
        --prefix="$OPUS_PREFIX" \
        --disable-shared \
        --enable-static \
        --disable-extra-programs \
        --disable-doc \
        CC="${CROSS_PREFIX}gcc"
    make -j"$(nproc)"
    make install
fi

cd "$PROJECT_DIR"
echo "Downloading and vendoring Go dependencies..."
"$GO_BIN" mod download
"$GO_BIN" mod vendor

if patch -p1 --dry-run < livekit-armv7.patch >/dev/null 2>&1; then
    patch -p1 < livekit-armv7.patch
elif patch -R -p1 --dry-run < livekit-armv7.patch >/dev/null 2>&1; then
    echo "ARMv7 patch is already applied."
else
    echo "ERROR: livekit-armv7.patch does not match the vendored dependencies." >&2
    exit 1
fi

mkdir -p build .gocache

echo "Building Luckfox LiveKit client..."
GOCACHE="$PROJECT_DIR/.gocache" \
PKG_CONFIG_PATH="$OPUS_PREFIX/lib/pkgconfig" \
PKG_CONFIG_LIBDIR="$OPUS_PREFIX/lib/pkgconfig" \
PKG_CONFIG_ALLOW_CROSS=1 \
CC="${CROSS_PREFIX}gcc" \
CGO_ENABLED=1 \
GOOS=linux \
GOARCH=arm \
GOARM=7 \
"$GO_BIN" build \
    -buildvcs=false \
    -mod=vendor \
    -tags nolibopusfile,nosoxr \
    -ldflags='-s -w' \
    -o build/luckfox-livekit-client .

file build/luckfox-livekit-client
ls -lh build/luckfox-livekit-client
echo "BUILD SUCCESS: $PROJECT_DIR/build/luckfox-livekit-client"
