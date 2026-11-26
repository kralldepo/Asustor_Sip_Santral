#!/bin/sh
# Simulates ADM App Central install / upgrade / uninstall of the APK inside
# a privileged Alpine container (BusyBox, like ADM). Run via:
#   docker run --rm --privileged -v "$PWD:/src:ro" alpine:3.22 sh /src/test/adm-sim.sh /src/dist/sipsantral_<ver>_arm64.apk
set -eu
APK=$1
REAL=/volume1/.@plugins/AppCentral/sipsantral
PKG=/usr/local/AppCentral/sipsantral
apk add -q curl unzip >/dev/null 2>&1 || true
mkdir -p /volume1/SipKayitlari /usr/local/AppCentral "$(dirname "$REAL")"
echo "korunacak kayıt" > /volume1/SipKayitlari/eski-kayit.mp3

unpack() {
	rm -rf /tmp/apk && mkdir -p /tmp/apk/ctrl "$REAL"
	unzip -q -o "$APK" -d /tmp/apk
	tar -xzf /tmp/apk/control.tar.gz -C /tmp/apk/ctrl
	tar -xzf /tmp/apk/data.tar.gz -C "$REAL"
	mkdir -p "$REAL/CONTROL" && cp /tmp/apk/ctrl/* "$REAL/CONTROL/"
	ln -sfn "$REAL" "$PKG"
}
run() { st=$1; f=$2; shift 2; APKG_PKG_DIR=$PKG APKG_PKG_STATUS=$st sh "$PKG/CONTROL/$f" "$@"; }
waitpanel() {
	i=0
	until curl -sk https://127.0.0.1:9490/api/status | grep -q setupRequired; do
		i=$((i + 1)); [ $i -lt 60 ] || { echo "PANEL AÇILMADI"; tail -20 $PKG/var/app.log; exit 1; }
		sleep 1
	done
}
waitasterisk() {
	i=0
	until ps | grep -v grep | grep -q "/usr/sbin/asterisk"; do
		i=$((i + 1)); [ $i -lt 90 ] || { echo "ASTERISK BAŞLAMADI"; tail -20 $PKG/var/app.log; exit 1; }
		sleep 1
	done
}
mounts() { grep -c "$REAL/rootfs" /proc/mounts || true; }

echo "== kurulum"
unpack
run install pre-install.sh
run install post-install.sh
run install start-stop.sh start
waitpanel; waitasterisk
sleep 3
echo "panel: $(curl -sk https://127.0.0.1:9490/api/status)"
echo "bağlı dosya sistemleri: $(mounts)"
curl -sk -c /tmp/j -H "X-Requested-With: sipsantral" -H "Content-Type: application/json" -d '{"password":"testparola1"}' https://127.0.0.1:9490/api/setup >/dev/null
curl -sk -b /tmp/j -H "X-Requested-With: sipsantral" -H "Content-Type: application/json" -d '{"num":"101","name":"Test","password":"Ext101Pass1"}' https://127.0.0.1:9490/api/extensions >/dev/null
echo "ayar: $(grep -c '"num": "101"' $PKG/var/config.json) dahili"

echo "== durdurma"
run install start-stop.sh stop
sleep 1
echo "asterisk süreci: $(ps | grep -v grep | grep -c /usr/sbin/asterisk || true), bağlı: $(mounts)"
[ "$(mounts)" = "0" ] || { echo "HATA: bağlı dosya sistemi kaldı"; exit 1; }

echo "== güncelleme"
run install start-stop.sh start; waitpanel; waitasterisk
run upgrade pre-uninstall.sh
run upgrade pre-install.sh
[ "$(mounts)" = "0" ] || { echo "HATA: güncellemede bağlı dosya sistemi kaldı"; exit 1; }
rm -rf "$REAL"            # App Central replaces the app folder
unpack
run upgrade post-install.sh
[ -f $PKG/var/config.json ] || { echo "HATA: ayarlar güncellemede kayboldu"; exit 1; }
echo "ayar korunmuş: $(grep -c '"num": "101"' $PKG/var/config.json) dahili"
run upgrade start-stop.sh start; waitpanel; waitasterisk
echo "güncelleme sonrası: $(curl -sk https://127.0.0.1:9490/api/status)"

echo "== kaldırma"
run uninstall pre-uninstall.sh
[ "$(mounts)" = "0" ] || { echo "HATA: kaldırmada bağlı dosya sistemi kaldı"; exit 1; }
rm -rf "$REAL" "$PKG"     # App Central removes the app folder
run uninstall post-uninstall.sh 2>/dev/null || APKG_PKG_DIR=$PKG APKG_PKG_STATUS=uninstall sh /tmp/apk/ctrl/post-uninstall.sh
ls /volume1/SipKayitlari/eski-kayit.mp3 >/dev/null && echo "kayıt klasörü korundu"
echo "TAMAM"
