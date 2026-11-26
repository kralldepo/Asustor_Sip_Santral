#!/bin/sh
# Runs inside an alpine:edge container (see build-rootfs.sh).
set -eu
ARCH=$1
MIRROR=https://dl-cdn.alpinelinux.org/alpine/edge
printf '%s/main\n%s/community\n%s/testing\n' "$MIRROR" "$MIRROR" "$MIRROR" > /etc/apk/repositories
apk update -q

R=/rootfs
rm -rf "$R"
mkdir -p "$R/etc/apk"
cp /etc/apk/repositories "$R/etc/apk/repositories"
cp -r /etc/apk/keys "$R/etc/apk/keys"

apk add -q --root "$R" --initdb --no-cache \
	alpine-baselayout busybox musl tzdata \
	asterisk lame libpiper espeak-ng

# The piper-tts package also pulls in Python; only its native CLI is needed.
cd /tmp
apk fetch -q --no-cache piper-tts
tar -xzf piper-tts-*.apk -C "$R" usr/bin/piper_exe

ASTERISK_VERSION=$(chroot "$R" /usr/sbin/asterisk -V | sed 's/^Asterisk //')
PIPER_VERSION=$(apk info -q --root "$R" libpiper 2>/dev/null | head -1)

# Trim: docs, headers, package caches, Asterisk's XML docs (saves RAM) and
# the sample configs (SIP Santral writes its own).
rm -rf "$R/usr/share/doc" "$R/usr/share/man" "$R/usr/share/info" "$R/usr/include" \
	"$R/var/cache/apk"/* "$R/var/lib/asterisk/documentation" "$R/var/lib/asterisk/static-http" \
	"$R/var/lib/asterisk/sounds"/* "$R/var/lib/asterisk/moh"/* "$R/etc/asterisk"/*
rm -rf "$R/etc/apk/keys" "$R/lib/apk/db/scripts.tar"
mkdir -p "$R/var/lib/asterisk/sounds/santral" "$R/var/spool/asterisk/monitor" \
	"$R/var/log/asterisk" "$R/run/asterisk" "$R/voices" "$R/santral" "$R/dev" "$R/proc" "$R/tmp"
chmod 1777 "$R/tmp"

cat > "$R/etc/santral-rootfs" <<EOF
arch=$ARCH
asterisk=$ASTERISK_VERSION
built=$(date -u +%Y-%m-%dT%H:%M:%SZ)
EOF
echo "asterisk: $ASTERISK_VERSION"
du -sh "$R"
tar -C "$R" -czf "/out/rootfs-$ARCH.tar.gz" .
