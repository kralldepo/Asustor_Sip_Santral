#!/bin/sh
# Builds the Alpine Linux root filesystem SIP Santral runs Asterisk in
# (chroot on the NAS): Asterisk 22, lame (MP3) and the native Piper TTS
# binary with its ONNX runtime. Needs Docker.
#
# Usage: rootfs/build-rootfs.sh [arm64|amd64]   ->  build/rootfs-<arch>.tar.gz
set -e
cd "$(dirname "$0")/.."
ARCH=${1:-arm64}
case $ARCH in
	arm64 | amd64) ;;
	*) echo "bilinmeyen mimari: $ARCH"; exit 1 ;;
esac
mkdir -p build
docker run --rm --platform "linux/$ARCH" \
	-v "$PWD/build:/out" -v "$PWD/rootfs:/src:ro" \
	alpine:edge sh /src/inside.sh "$ARCH"
ls -la "build/rootfs-$ARCH.tar.gz"
