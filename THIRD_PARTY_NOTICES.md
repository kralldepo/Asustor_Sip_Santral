# Third-Party Notices / Üçüncü Taraf Bildirimleri

**Türkçe özet:** SIP Santral'ın kendi kaynak kodu [MIT lisansı](LICENSE) ile yayınlanır. Hazır APK dosyası ise kendi lisansları olan açık kaynak bileşenler içerir (Asterisk, BusyBox, PJSIP ve eSpeak NG gibi GPL lisanslı bileşenler dahil). Bu bileşenler değiştirilmeden Alpine Linux paketlerinden alınır ve MIT lisansına girmez; her biri aşağıda belirtilen kendi lisansına tabidir. Kaynak kodlarına nasıl ulaşılacağı [aşağıda](#corresponding-source-code) anlatılmıştır.

---

The source code in this repository is licensed under the [MIT License](LICENSE). The prebuilt APK published under [Releases](https://github.com/kralldepo/Asustor_Sip_Santral/releases) additionally contains third-party software that is **not** covered by the MIT License. Each component remains under its own license, listed below. All third-party components are used unmodified.

The APK consists of two parts:

1. `bin/sipsantral`: the SIP Santral program, compiled from this repository with Go.
2. `rootfs.tar.gz`: an Alpine Linux root file system that contains Asterisk and its dependencies. It is built by [`rootfs/build-rootfs.sh`](rootfs/build-rootfs.sh) from official Alpine Linux **edge** binary packages.

## 1. Components compiled into `bin/sipsantral`

| Component | Version | License |
|---|---|---|
| [Go standard library and runtime](https://go.dev) | Go 1.26+ | BSD-3-Clause |
| [golang.org/x/crypto](https://pkg.go.dev/golang.org/x/crypto) (bcrypt) | v0.59.0 | BSD-3-Clause |
| [golang.org/x/crypto/x509roots/fallback](https://pkg.go.dev/golang.org/x/crypto/x509roots/fallback) | v0.0.0-20261115193255-739070949446 | BSD-3-Clause; embeds the Mozilla CA root list (MPL-2.0) |

## 2. Packages in `rootfs.tar.gz` (APK 1.0.1, arm64)

Versions and licenses below are taken from the Alpine package database inside the root file system (`/lib/apk/db/installed`), built on 2026-11-19. License identifiers are [SPDX](https://spdx.org/licenses/) expressions as declared by Alpine.

| Alpine source package | Version | License | Upstream |
|---|---|---|---|
| abseil-cpp (48 library sub-packages) | 20260817.0-r0 | Apache-2.0 | https://abseil.io/ |
| alpine-baselayout | 3.7.2-r1 | GPL-2.0-only | https://gitlab.alpinelinux.org/alpine/aports |
| alsa-lib | 1.2.16.1-r0 | LGPL-2.1-or-later | https://alsa-project.org/ |
| **asterisk** | 22.11.0-r1 | GPL-2.0-only WITH Asterisk-linking-protocols-exception | https://www.asterisk.org/ |
| brotli (brotli-libs) | 1.2.1-r0 | MIT | https://github.com/google/brotli |
| busybox (busybox, busybox-binsh, ssl_client) | 1.38.0-r8 | GPL-2.0-only | https://busybox.net/ |
| c-ares | 1.34.9-r0 | MIT | https://c-ares.org/ |
| ca-certificates (ca-certificates-bundle) | 20261020-r0 | MPL-2.0 AND MIT | https://www.mozilla.org/about/governance/policies/security-group/certs/ |
| curl (libcurl) | 8.23.0-r0 | curl | https://curl.se/ |
| dbus (dbus-libs) | 1.16.4-r0 | AFL-2.1 OR GPL-2.0-or-later | https://www.freedesktop.org/wiki/Software/dbus/ |
| **espeak-ng** | 1.52.0-r1 | GPL-3.0-or-later | https://github.com/espeak-ng/espeak-ng |
| gcc (libgcc, libstdc++) | 15.2.0-r9 | GPL-2.0-or-later AND LGPL-2.1-or-later (with GCC Runtime Library Exception) | https://gcc.gnu.org |
| gsm | 1.0.24-r0 | TU-Berlin-2.0 | https://www.quut.com/gsm/ |
| hiredis | 1.3.0-r0 | BSD-3-Clause | https://github.com/redis/hiredis |
| icu (icu-libs, icu-data-en) | 78.3-r0 | ICU | https://icu.unicode.org/ |
| imap (c-client) | 2007f-r16 | Apache-2.0 | http://www.washington.edu/imap |
| jack | 1.9.22-r4 | GPL-2.0-or-later | https://jackaudio.org/ |
| jansson | 2.15.1-r0 | MIT | http://www.digip.org/jansson/ |
| **lame** | 3.100-r5 | LGPL-2.0-or-later | https://lame.sourceforge.net/ |
| libcap (libcap2) | 2.78-r1 | BSD-3-Clause OR GPL-2.0-only | https://sites.google.com/site/fullycapable/ |
| libedit | 20260512.3.1-r0 | BSD-3-Clause | https://www.thrysoee.dk/editline |
| libevent | 2.1.13-r0 | BSD-3-Clause | https://libevent.org/ |
| libidn2 | 2.3.8-r0 | GPL-2.0-or-later OR LGPL-3.0-or-later | https://www.gnu.org/software/libidn/#libidn2 |
| libpsl | 0.21.5-r3 | MIT | https://rockdaboot.github.io/libpsl |
| libsamplerate | 0.2.2-r3 | BSD-2-Clause | http://www.mega-nerd.com/SRC/index.html |
| libsrtp | 2.8.0-r0 | BSD-3-Clause | https://github.com/cisco/libsrtp |
| libunistring | 1.4.2-r0 | GPL-2.0-or-later OR LGPL-3.0-or-later | https://www.gnu.org/software/libunistring/ |
| libxml2 | 2.13.9-r3 | MIT | https://gitlab.gnome.org/GNOME/libxml2 |
| lua5.1 (lua5.1-libs) | 5.1.5-r13 | MIT | https://www.lua.org/ |
| musl | 1.2.6-r6 | MIT | https://musl.libc.org/ |
| ncurses (libncursesw, ncurses-terminfo-base) | 6.6_p20261114-r0 | X11 | https://invisible-island.net/ncurses/ |
| nghttp2 (nghttp2-libs) | 1.71.0-r0 | MIT | https://nghttp2.org |
| onnxruntime | 1.30.0-r0 | MIT | https://onnxruntime.ai/ |
| openssl (libcrypto3, libssl3) | 3.5.9-r1 | Apache-2.0 | https://www.openssl.org/ |
| opus | 1.6.1-r0 | BSD-3-Clause | https://www.opus-codec.org/ |
| pcaudiolib | 1.3-r0 | GPL-3.0-or-later | https://github.com/espeak-ng/pcaudiolib |
| **piper-tts** (libpiper, `piper_exe`) | 1.7.0-r0 | GPL-3.0 (see note) | https://github.com/OHF-Voice/piper1-gpl |
| **pjproject** | 2.17-r0 | GPL-2.0-or-later | https://www.pjsip.org/ |
| portaudio | 19.7.0-r3 | MIT | http://www.portaudio.com/ |
| protobuf (libprotobuf, libprotobuf-lite, libprotoc, protobuf, protoc) | 31.1-r4 | BSD-3-Clause | https://github.com/protocolbuffers/protobuf |
| protobuf-c | 1.5.2-r4 | BSD-2-Clause | https://github.com/protobuf-c/protobuf-c |
| re2 | 2025.11.05-r5 | BSD-3-Clause | https://github.com/google/re2 |
| speex | 1.2.1-r2 | BSD-3-Clause | https://www.speex.org/ |
| speexdsp | 1.2.1-r2 | BSD-3-Clause | https://www.speex.org/ |
| sqlite (sqlite-libs) | 3.54.1-r0 | blessing | https://www.sqlite.org/ |
| tzdata | 2026f-r0 | Public-Domain | https://www.iana.org/time-zones |
| unbound (unbound-libs) | 1.26.2-r0 | BSD-3-Clause | https://nlnetlabs.nl/projects/unbound/about/ |
| util-linux (libuuid) | 2.42.4-r3 | BSD-3-Clause | https://git.kernel.org/pub/scm/utils/util-linux/util-linux.git |
| xz (xz-libs) | 5.8.4-r0 | GPL-2.0-or-later AND 0BSD AND Public-Domain AND LGPL-2.1-or-later | https://tukaani.org/xz/ |
| zlib | 1.4-r0 | Zlib | https://zlib.net/ |
| zstd (zstd-libs) | 1.5.7-r2 | BSD-3-Clause OR GPL-2.0-or-later | https://facebook.github.io/zstd/ |

**Note on Piper:** Alpine's package metadata lists `piper-tts` as MIT, but the upstream project ([OHF-Voice/piper1-gpl](https://github.com/OHF-Voice/piper1-gpl)) is published under GPL-3.0 and embeds eSpeak NG (GPL-3.0-or-later). This project therefore treats Piper as GPL-3.0.

Asterisk's own sound files and sample configuration files are removed from the root file system; SIP Santral generates its configuration and its beep tone itself.

The APK runs these programs as separate processes (Asterisk inside a chroot, Piper as a command-line tool). SIP Santral communicates with Asterisk over the Asterisk Manager Interface network protocol and does not link against any GPL code.

## 3. Software downloaded at run time (not distributed)

- **Piper voice models** are downloaded by the user, on demand, from [rhasspy/piper-voices](https://huggingface.co/rhasspy/piper-voices) on Hugging Face (v1.0.0): `tr_TR-fahrettin-medium`, `tr_TR-fettah-medium`, `tr_TR-dfki-medium`, `de_DE-thorsten-medium`, `de_DE-kerstin-low`, `en_US-amy-medium`. Each voice comes with a `MODEL_CARD` that names its training data set and license. Some data sets restrict commercial use; check the model card before using a voice commercially.

## Corresponding source code

The complete corresponding source code for every package listed in section 2 (build recipes, patches and the upstream source archives) is published by the Alpine Linux project:

- Build recipes (`APKBUILD`) and patches: [aports repository](https://gitlab.alpinelinux.org/alpine/aports), directories `main/<package>`, `community/<package>` or `testing/<package>`. The package version above identifies the matching recipe in the repository history.
- Upstream source archives: the upstream links in the table above.
- The exact steps used to assemble the root file system: [`rootfs/build-rootfs.sh`](rootfs/build-rootfs.sh) and [`rootfs/inside.sh`](rootfs/inside.sh) in this repository.

If you need the complete corresponding source code of a GPL- or LGPL-licensed component exactly as used in a published release and cannot get it from the links above, open an issue in this repository. It will be provided for at least three years after the release that contains the component.

## License texts

Full license texts are available from the [SPDX License List](https://spdx.org/licenses/) and in each upstream project. The Asterisk linking exception is documented by the Asterisk project and listed by SPDX as `Asterisk-linking-protocols-exception`.
