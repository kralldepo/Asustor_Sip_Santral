#!/bin/sh
# Before install/upgrade: on upgrade stop the service and move settings,
# announcements, voices and the call history out of the app folder so they
# survive the replacement of the app files. Recordings live in the
# SipKayitlari shared folder and are never touched.

# ---- shared helpers (same block in every lifecycle script) ----
PKG_NAME=sipsantral
PKG_DIR="${APKG_PKG_DIR:-/usr/local/AppCentral/$PKG_NAME}"
PIDFILE="/var/run/$PKG_NAME.pid"
BAK="$(dirname "$PKG_DIR")/$PKG_NAME-var-upgrade"

real_pkg_dir() {
	readlink -f "$PKG_DIR" 2>/dev/null || echo "$PKG_DIR"
}

# Kill processes still running inside the Asterisk root file system (only
# needed when the panel itself was killed before it could stop Asterisk).
kill_chroot_procs() {
	root="$(real_pkg_dir)/rootfs"
	for p in /proc/[0-9]*; do
		r=$(readlink "$p/root" 2>/dev/null) || continue
		if [ "$r" = "$root" ]; then
			kill -9 "${p#/proc/}" 2>/dev/null
		fi
	done
}

# Detach /proc and /dev mounted inside the root file system so that a later
# removal of the app folder can never reach into them.
unmount_rootfs() {
	for root in "$PKG_DIR/rootfs" "$(real_pkg_dir)/rootfs"; do
		awk -v r="$root" '$2 == r || index($2, r "/") == 1 { print $2 }' /proc/mounts 2>/dev/null | sort -r | while read -r mp; do
			umount "$mp" 2>/dev/null || umount -l "$mp" 2>/dev/null
		done
	done
}

stop_service() {
	if [ -f "$PIDFILE" ]; then
		pid=$(cat "$PIDFILE" 2>/dev/null)
		if [ -n "$pid" ] && kill -0 "$pid" 2>/dev/null; then
			kill -TERM "$pid" 2>/dev/null
			i=0
			while [ $i -lt 45 ] && kill -0 "$pid" 2>/dev/null; do
				sleep 1
				i=$((i + 1))
			done
			kill -KILL "$pid" 2>/dev/null
		fi
		rm -f "$PIDFILE"
	fi
	kill_chroot_procs
	unmount_rootfs
}
# ---- end of helpers ----

if [ "$APKG_PKG_STATUS" = "upgrade" ]; then
	stop_service
	if [ -d "$PKG_DIR/var" ]; then
		rm -rf "$BAK"
		mv "$PKG_DIR/var" "$BAK"
	fi
fi
exit 0
