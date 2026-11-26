#!/bin/sh
# Builds the ASUSTOR App Central package dist/sipsantral_<version>_<arch>.apk
#   ./build.sh            ARM64 (AS11xx, AS33xx, AS40xx ... e.g. AS1104T)
#   ./build.sh amd64      x86-64 models (needs Docker with amd64 emulation)
# The Asterisk root file system (build/rootfs-<arch>.tar.gz) is built once
# with Docker by rootfs/build-rootfs.sh; delete it to rebuild.
set -e
cd "$(dirname "$0")"

VERSION=$(sed -n 's/^var version = "\(.*\)"/\1/p' main.go)
[ -n "$VERSION" ] || { echo "sürüm bulunamadı"; exit 1; }

for goarch in ${1:-arm64}; do
	case $goarch in
		arm64) apkarch=arm64 ;;
		amd64) apkarch=x86-64 ;;
		*) echo "bilinmeyen mimari: $goarch"; exit 1 ;;
	esac
	[ -f "build/rootfs-$goarch.tar.gz" ] || ./rootfs/build-rootfs.sh "$goarch"
	stage="build/stage-$apkarch"
	rm -rf "$stage"
	mkdir -p "$stage/bin"
	cp -R package/CONTROL "$stage/CONTROL"
	sed -e "s/__VERSION__/$VERSION/" -e "s/__ARCH__/$apkarch/" package/CONTROL/config.json > "$stage/CONTROL/config.json"
	echo "==> $apkarch derleniyor (v$VERSION)"
	CGO_ENABLED=0 GOOS=linux GOARCH=$goarch go build -trimpath -ldflags "-s -w -X main.version=$VERSION" -o "$stage/bin/sipsantral" .
	cp "build/rootfs-$goarch.tar.gz" "$stage/rootfs.tar.gz"
	go run ./tools/mkapk "$stage" dist
done
ls -la dist/
