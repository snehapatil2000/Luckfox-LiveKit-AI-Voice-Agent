#!/usr/bin/env bash
set -euo pipefail
script_dir="$(cd -- "$(dirname -- "$0")" && pwd)"
client_dir="$script_dir/../reference-code/luckfox-client"
output="$client_dir/build/microwakeword"
cache="$output/sources"
revision=96bd69cfad79aa67697e176570d3dd87052c3def
compiler="${MICROWAKEWORD_CXX:-arm-linux-gnueabihf-g++}"
runtime="${MICROWAKEWORD_GLIBC_DIR:-/usr/arm-linux-gnueabihf/lib}"
command -v "$compiler" >/dev/null
mkdir -p "$cache" "$output/lib" "$output/licenses"
wheel="$cache/runtime-2.5.0-armv7l.whl"
frontend="$cache/features-$revision.zip"
[[ -s "$wheel" ]] || curl -fL --retry 2 \
    https://files.pythonhosted.org/packages/30/1e/11cf9d78df0a8881bc75de9b322f901403e1315cae9465c9aafc0f9c91ef/pymicro_wakeword-2.5.0-py3-none-manylinux_2_35_armv7l.whl -o "$wheel"
[[ -s "$frontend" ]] || curl -fL --retry 2 \
    "https://codeload.github.com/rhasspy/pymicro-features/zip/$revision" -o "$frontend"
printf '%s  %s\n' af92b862fe0488fba7422b9819b1a8c88781b2f90fb3e4b7f1dcfd1d90a6b69b "$wheel" \
    f5d5ae6230605a9e57cc6f625e10428a41e7f0fdc1683adcfbaa9d83030a403c "$frontend" | sha256sum -c -
python3 -m zipfile -e "$wheel" "$cache/runtime"
python3 -m zipfile -e "$frontend" "$cache/frontend"
features="$cache/frontend/pymicro-features-$revision"
micro="$features/tensorflow/lite/experimental/microfrontend/lib"
sources=()
for name in kiss_fft_int16 fft fft_util filterbank filterbank_util frontend frontend_util log_lut log_scale log_scale_util noise_reduction noise_reduction_util pcan_gain_control pcan_gain_control_util window window_util; do
    sources+=("$micro/$name.cc")
done
cp "$cache/runtime/pymicro_wakeword/lib/libtensorflowlite_c.so" "$output/lib/"
cp "$cache/runtime/pymicro_wakeword/models/alexa.tflite" "$cache/runtime/pymicro_wakeword/models/alexa.json" "$output/"
cp "$features/LICENSE" "$output/licenses/micro-features-LICENSE"
cp "$cache/runtime/pymicro_wakeword-2.5.0.dist-info/licenses/LICENSE" "$output/licenses/micro-wakeword-LICENSE"
# The vendored Kiss FFT license is embedded in its source header.
cp "$features/kissfft/kiss_fft.cc" "$output/licenses/kiss_fft.cc"
for library in ld-linux-armhf.so.3 libc.so.6 libm.so.6 libstdc++.so.6 libgcc_s.so.1; do
    cp -L "$runtime/$library" "$output/lib/$library"
done
"$compiler" -std=c++11 -O3 -Wall -Wextra -DFIXED_POINT=16 \
    -I"$features" -I"$features/kissfft" "$client_dir/wakeword/microwakeword_stream.cpp" \
    "${sources[@]}" "$features/kissfft/kiss_fft.cc" "$features/kissfft/tools/kiss_fftr.cc" \
    -L"$output/lib" -Wl,-rpath-link,"$output/lib" -l:libtensorflowlite_c.so -o "$output/microwakeword-stream"
cp "$client_dir/deploy/run-microwakeword.sh" "$output/"
chmod 755 "$output/run-microwakeword.sh" "$output/microwakeword-stream"
file "$output/microwakeword-stream"
echo "Built microWakeWord with the Alexa model: $output"
echo "Deploy the bundle without the sources directory."
