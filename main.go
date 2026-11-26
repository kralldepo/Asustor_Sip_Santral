package main

import (
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	_ "golang.org/x/crypto/x509roots/fallback" // CA roots when the NAS has none
	_ "time/tzdata"                            // time zones when the NAS has no zoneinfo
)

var version = "1.0.1"

func defaultPkgDir() string {
	if d := os.Getenv("APKG_PKG_DIR"); d != "" {
		return d
	}
	if st, err := os.Stat("/usr/local/AppCentral/sipsantral"); err == nil && st.IsDir() {
		return "/usr/local/AppCentral/sipsantral"
	}
	return "."
}

func main() {
	pkgDir := flag.String("pkg", defaultPkgDir(), "uygulama klasörü (rootfs.tar.gz burada)")
	dataDir := flag.String("data", "", "veri klasörü (varsayılan: <pkg>/var)")
	listen := flag.String("listen", ":9490", "panel adresi")
	plainHTTP := flag.Bool("http", false, "TLS olmadan HTTP (yalnız test için)")
	noPBX := flag.Bool("no-pbx", false, "Asterisk'i başlatma (arayüz geliştirme)")
	certFile := flag.String("cert", "/usr/builtin/etc/certificate/ssl.crt", "TLS sertifikası")
	keyFile := flag.String("key", "/usr/builtin/etc/certificate/ssl.key", "TLS anahtarı")
	pemFile := flag.String("pem", "/usr/builtin/etc/certificate/ssl.pem", "TLS sertifika+anahtar (yedek)")
	uiDir := flag.String("ui-dir", "", "geliştirme: arayüzü bu klasörden sun")
	showVersion := flag.Bool("version", false, "sürümü yazdır")
	flag.Parse()
	if *showVersion {
		fmt.Println(version)
		return
	}
	if *dataDir == "" {
		*dataDir = filepath.Join(*pkgDir, "var")
	}
	if err := os.MkdirAll(*dataDir, 0o700); err != nil {
		log.Fatalf("veri klasörü oluşturulamadı: %v", err)
	}
	_ = os.Chmod(*dataDir, 0o700)
	for _, d := range []string{"sounds", "voices", "calls"} {
		_ = os.MkdirAll(filepath.Join(*dataDir, d), 0o755)
	}
	logw, err := newRotatingWriter(filepath.Join(*dataDir, "app.log"), 5<<20)
	if err != nil {
		log.Fatalf("log açılamadı: %v", err)
	}
	var out io.Writer = logw
	if fi, err := os.Stdout.Stat(); err == nil && fi.Mode()&os.ModeCharDevice != 0 {
		out = io.MultiWriter(logw, os.Stdout)
	}
	lg := log.New(out, "", log.LstdFlags)
	lg.Printf("SIP Santral %s başlıyor (paket: %s, veri: %s)", version, *pkgDir, *dataDir)

	store, err := OpenStore(filepath.Join(*dataDir, "config.json"))
	if err != nil {
		lg.Fatalf("ayarlar okunamadı: %v", err)
	}
	calls, err := OpenCallLog(filepath.Join(*dataDir, "calls"))
	if err != nil {
		lg.Fatalf("arama geçmişi okunamadı: %v", err)
	}
	calls.Compact()
	events := OpenEventLog(filepath.Join(*dataDir, "events.jsonl"))

	pbx := NewPBX(*pkgDir, *dataDir, store, calls, events, lg, !*noPBX)
	store.OnChange(pbx.RequestReload)
	tts := NewTTS(pbx, lg)

	ctx, stop := context.WithCancel(context.Background())
	pbxDone := make(chan struct{})
	go func() {
		pbx.Run(ctx)
		close(pbxDone)
	}()

	web := &Web{store: store, pbx: pbx, calls: calls, tts: tts, ev: events, dataDir: *dataDir, version: version, uiDir: *uiDir, log: lg}
	srv := &http.Server{
		Handler:           web.Handler(),
		ReadHeaderTimeout: 15 * time.Second,
		IdleTimeout:       120 * time.Second,
		ErrorLog:          log.New(io.Discard, "", 0),
	}
	ln, err := net.Listen("tcp", *listen)
	if err != nil {
		lg.Fatalf("%s dinlenemiyor: %v", *listen, err)
	}
	go func() {
		var serr error
		if *plainHTTP {
			lg.Printf("HTTP dinleniyor: %s (TLS kapalı!)", *listen)
			serr = srv.Serve(ln)
		} else {
			cm := &certManager{certFile: *certFile, keyFile: *keyFile, pemFile: *pemFile, selfDir: filepath.Join(*dataDir, "tls"), log: lg}
			srv.TLSConfig = &tls.Config{GetCertificate: cm.GetCertificate, MinVersion: tls.VersionTLS12}
			lg.Printf("HTTPS dinleniyor: %s", *listen)
			serr = srv.ServeTLS(ln, "", "")
		}
		if serr != nil && !errors.Is(serr, http.ErrServerClosed) {
			lg.Fatalf("web sunucusu: %v", serr)
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	s := <-sig
	lg.Printf("sinyal alındı (%v), kapanıyor", s)
	stop()
	shCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	_ = srv.Shutdown(shCtx)
	cancel()
	select {
	case <-pbxDone:
	case <-time.After(25 * time.Second):
		lg.Printf("Asterisk zamanında durmadı")
	}
	unmountAll(filepath.Join(*pkgDir, "rootfs"), lg)
	lg.Printf("durdu")
}
