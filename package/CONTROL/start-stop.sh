#!/bin/sh
# SIP Santral - ADM App Central start/stop script (BusyBox sh)

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
BIN="$PKG_DIR/bin/sipsantral"
DATA="$PKG_DIR/var"
PORT=9490

is_running() {
	[ -f "$PIDFILE" ] || return 1
	pid=$(cat "$PIDFILE" 2>/dev/null)
	[ -n "$pid" ] || return 1
	kill -0 "$pid" 2>/dev/null || return 1
	if [ -r "/proc/$pid/cmdline" ]; then
		tr '\0' ' ' < "/proc/$pid/cmdline" | grep -q "bin/sipsantral" || return 1
	fi
	return 0
}

start() {
	if is_running; then
		echo "$PKG_NAME already running"
		return 0
	fi
	kill_chroot_procs
	unmount_rootfs
	mkdir -p "$DATA" && chmod 700 "$DATA"
	if [ -f "$DATA/stderr.log" ] && [ "$(wc -c < "$DATA/stderr.log")" -gt 1048576 ]; then
		mv -f "$DATA/stderr.log" "$DATA/stderr.log.1"
	fi
	cd "$PKG_DIR" || return 1
	ulimit -n 8192 2>/dev/null
	APKG_PKG_DIR="$PKG_DIR" nohup "$BIN" -pkg "$PKG_DIR" -data "$DATA" -listen ":$PORT" >> "$DATA/stderr.log" 2>&1 < /dev/null &
	echo $! > "$PIDFILE"
	sleep 1
	if is_running; then
		echo "$PKG_NAME started"
		return 0
	fi
	echo "$PKG_NAME failed to start, see $DATA/stderr.log"
	return 1
}

stop() {
	stop_service
	echo "$PKG_NAME stopped"
	return 0
}

case "$1" in
	start)
		start
		;;
	stop)
		stop
		;;
	restart|reload)
		stop
		start
		;;
	status)
		if is_running; then
			echo "running"
			exit 0
		fi
		echo "stopped"
		exit 1
		;;
	*)
		echo "Usage: $0 {start|stop|restart|status}"
		exit 1
		;;
esac
exit $?
