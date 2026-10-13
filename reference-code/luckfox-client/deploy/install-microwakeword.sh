#!/bin/sh
# Run on Luckfox with a staged runtime bundle and a matching client binary.
set -eu
umask 077
stage=${1:?Usage: install-microwakeword.sh STAGING_DIRECTORY}
for file in luckfox-livekit-client microwakeword-stream run-microwakeword.sh alexa.tflite alexa.json lib/ld-linux-armhf.so.3 lib/libtensorflowlite_c.so; do
    test -s "$stage/$file"
done
test -f /root/luckfox-livekit.env
archive=$(mktemp -d /root/wake-detector-archive.XXXXXX)
cp -p /root/luckfox-livekit.env "$archive/luckfox-livekit.env"
cp -p /root/luckfox-livekit-client "$archive/luckfox-livekit-client"
/etc/init.d/S99zz-livekit-voice stop
sleep 2
rollback() {
    /etc/init.d/S99zz-livekit-voice stop
    cp -p "$archive/luckfox-livekit.env" /root/luckfox-livekit.env
    cp -p "$archive/luckfox-livekit-client" /root/luckfox-livekit-client
    /etc/init.d/S99zz-livekit-voice start
    echo "Installation failed. Previous client restored. Backup: $archive" >&2
}
trap rollback EXIT
mkdir -p /opt/luckfox-microwakeword
cp "$stage/microwakeword-stream" "$stage/run-microwakeword.sh" "$stage/alexa.tflite" "$stage/alexa.json" /opt/luckfox-microwakeword/
cp -R "$stage/lib" "$stage/licenses" /opt/luckfox-microwakeword/
chmod 755 /opt/luckfox-microwakeword/run-microwakeword.sh /opt/luckfox-microwakeword/microwakeword-stream
/opt/luckfox-microwakeword/run-microwakeword.sh /opt/luckfox-microwakeword/alexa.tflite alexa 0.9 5 </dev/null
cp "$stage/luckfox-livekit-client" /root/luckfox-livekit-client
chmod 755 /root/luckfox-livekit-client
# Replace only detector configuration. Preserve all cloud and gateway credentials.
awk '!/^(export )?(WAKE_WORD|WAKEWORD_BINARY|WAKEWORD_MODEL|WAKEWORD_CONFIG|WAKEWORD_DICTIONARY|WAKEWORD_THRESHOLD)=/ {print}
END {
    print "WAKE_WORD=alexa"
    print "WAKEWORD_BINARY=/opt/luckfox-microwakeword/run-microwakeword.sh"
    print "WAKEWORD_MODEL=/opt/luckfox-microwakeword/alexa.tflite"
    print "WAKEWORD_CONFIG=/opt/luckfox-microwakeword/alexa.json"
}' "$archive/luckfox-livekit.env" > /root/luckfox-livekit.env
chmod 600 /root/luckfox-livekit.env
/etc/init.d/S99zz-livekit-voice start
trap - EXIT
# Remove inactive detector installations from their former locations, recoverably.
for legacy in /opt/luckfox-wakeword /root/edison-neural-test /root/porcupine-test; do
    if [ -d "$legacy" ]; then
        mv "$legacy" "$archive/"
    fi
done
echo "Alexa runtime installed. Previous detector files and configuration: $archive"
