#!/bin/sh
set -eu
detector_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
exec "$detector_dir/lib/ld-linux-armhf.so.3" \
    --library-path "$detector_dir/lib" "$detector_dir/microwakeword-stream" "$@"
