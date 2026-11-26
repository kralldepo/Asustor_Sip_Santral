package main

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

//go:embed ui
var uiFS embed.FS

const (
	sessionCookie = "ss_session"
	sessionTTL    = 12 * time.Hour
	csrfHeader    = "X-Requested-With"
	csrfValue     = "sipsantral"
)

type Web struct {
	store   *Store
	pbx     *PBX
	calls   *CallLog
	tts     *TTS
	ev      *EventLog
	dataDir string
	version string
	uiDir   string
	log     *log.Logger

	smu       sync.Mutex
	sessSaved time.Time
	sessions  map[string]time.Time
	fails     map[string]*loginFail
}

type loginFail struct {
	count        int
	first        time.Time
	blockedUntil time.Time
}

func (w *Web) Handler() http.Handler {
	if w.sessions == nil {
		w.sessions = map[string]time.Time{}
		w.loadSessions()
	}
	if w.fails == nil {
		w.fails = map[string]*loginFail{}
	}
	mux := http.NewServeMux()
	var sub fs.FS
	if w.uiDir != "" {
		sub = os.DirFS(w.uiDir)
	} else {
		sub, _ = fs.Sub(uiFS, "ui")
	}
	static := http.FileServer(http.FS(sub))
	mux.HandleFunc("GET /{$}", func(rw http.ResponseWriter, r *http.Request) {
		b, err := fs.ReadFile(sub, "index.html")
		if err != nil {
			http.Error(rw, "index.html yok", 500)
			return
		}
		rw.Header().Set("Content-Type", "text/html; charset=utf-8")
		rw.Header().Set("Cache-Control", "no-cache")
		rw.Write(b)
	})
	uiFiles := http.StripPrefix("/ui/", static)
	mux.HandleFunc("GET /ui/", func(rw http.ResponseWriter, r *http.Request) {
		rw.Header().Set("Cache-Control", "no-cache")
		uiFiles.ServeHTTP(rw, r)
	})
	mux.HandleFunc("GET /favicon.ico", func(rw http.ResponseWriter, r *http.Request) {
		b, err := fs.ReadFile(sub, "icon.png")
		if err != nil {
			http.NotFound(rw, r)
			return
		}
		rw.Header().Set("Content-Type", "image/png")
		rw.Write(b)
	})

	api := func(pattern string, h func(http.ResponseWriter, *http.Request)) {
		mux.HandleFunc(pattern, w.requireAuth(h))
	}
	mux.HandleFunc("GET /api/status", w.handleStatus)
	mux.HandleFunc("POST /api/setup", w.csrf(w.handleSetup))
	mux.HandleFunc("POST /api/login", w.csrf(w.handleLogin))
	api("POST /api/logout", w.handleLogout)
	api("POST /api/password", w.handlePassword)

	api("GET /api/dashboard", w.handleDashboard)

	api("GET /api/lines", w.handleLines)
	api("GET /api/lines/{id}", w.handleLineGet)
	api("POST /api/lines", w.handleLineSave)
	api("PUT /api/lines/{id}", w.handleLineSave)
	api("DELETE /api/lines/{id}", w.handleLineDelete)
	api("POST /api/lines/{id}/mode", w.handleLineMode)
	api("POST /api/lines/{id}/enabled", w.handleLineEnabled)
	api("POST /api/lines/{id}/test", w.handleLineTest)
	api("POST /api/lines/import", w.handleLineImport)
	api("POST /api/lines/bulk", w.handleLinesBulk)

	api("GET /api/extensions", w.handleExts)
	api("POST /api/extensions", w.handleExtSave)
	api("PUT /api/extensions/{num}", w.handleExtSave)
	api("DELETE /api/extensions/{num}", w.handleExtDelete)
	api("GET /api/extensions/{num}/secret", w.handleExtSecret)
	api("POST /api/extensions/{num}/password", w.handleExtResetPassword)

	api("GET /api/announcements", w.handleAnns)
	api("POST /api/announcements", w.handleAnnCreate)
	api("PUT /api/announcements/{id}", w.handleAnnRename)
	api("DELETE /api/announcements/{id}", w.handleAnnDelete)
	api("GET /api/announcements/{id}/audio", w.handleAnnAudio)
	api("POST /api/tts/preview", w.handleTTSPreview)
	api("POST /api/tts/voices/{id}/download", w.handleVoiceDownload)
	api("DELETE /api/tts/voices/{id}", w.handleVoiceDelete)

	api("GET /api/calls", w.handleCalls)
	api("GET /api/calls/export.csv", w.handleCallsCSV)
	api("POST /api/calls/zip", w.handleCallsZip)
	api("GET /api/calls/{id}/audio", w.handleCallAudio)

	api("GET /api/settings", w.handleSettingsGet)
	api("PUT /api/settings", w.handleSettingsPut)
	api("POST /api/system/restart", w.handleRestart)
	api("POST /api/system/sipdebug", w.handleSIPDebug)
	api("POST /api/system/unblock", w.handleUnblock)
	api("GET /api/system/log", w.handleLog)
	api("GET /api/backup", w.handleBackupGet)
	api("POST /api/backup", w.handleBackupRestore)

	return w.secure(mux)
}

// ---------------------------------------------------------------------------
// Middleware
// ---------------------------------------------------------------------------

func (w *Web) secure(next http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		h := rw.Header()
		host := r.Host
		if hh, _, err := net.SplitHostPort(r.Host); err == nil {
			host = hh
		}
		frame := "'self'"
		if host != "" && !strings.ContainsAny(host, " ;'\"") {
			if strings.Contains(host, ":") {
				host = "[" + host + "]"
			}
			frame += " https://" + host + ":* http://" + host + ":*"
		}
		h.Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; media-src 'self' blob:; style-src 'self'; script-src 'self'; connect-src 'self'; base-uri 'none'; form-action 'self'; frame-ancestors "+frame)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Permissions-Policy", "microphone=(self)")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			h.Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(rw, r)
	})
}

func (w *Web) csrf(next http.HandlerFunc) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			if r.Header.Get(csrfHeader) != csrfValue {
				jsonErr(rw, http.StatusForbidden, "geçersiz istek")
				return
			}
			if o := r.Header.Get("Origin"); o != "" {
				u, err := url.Parse(o)
				if err != nil || u.Host != r.Host {
					jsonErr(rw, http.StatusForbidden, "geçersiz kaynak")
					return
				}
			}
		}
		next(rw, r)
	}
}

func (w *Web) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return w.csrf(func(rw http.ResponseWriter, r *http.Request) {
		if !w.authed(r) {
			jsonErr(rw, http.StatusUnauthorized, "oturum gerekli")
			return
		}
		next(rw, r)
	})
}

func (w *Web) authed(r *http.Request) bool {
	c, err := r.Cookie(sessionCookie)
	if err != nil || c.Value == "" {
		return false
	}
	key := sessKey(c.Value)
	w.smu.Lock()
	defer w.smu.Unlock()
	exp, ok := w.sessions[key]
	if !ok || time.Now().After(exp) {
		if ok {
			delete(w.sessions, key)
			w.saveSessionsLocked()
		}
		return false
	}
	w.sessions[key] = time.Now().Add(sessionTTL)
	if time.Since(w.sessSaved) > 10*time.Minute {
		w.saveSessionsLocked()
	}
	return true
}

func sessKey(tok string) string {
	h := sha256.Sum256([]byte(tok))
	return hex.EncodeToString(h[:])
}

func (w *Web) sessionsPath() string { return filepath.Join(w.dataDir, "sessions.json") }

func (w *Web) loadSessions() {
	b, err := os.ReadFile(w.sessionsPath())
	if err != nil {
		return
	}
	var m map[string]int64
	if json.Unmarshal(b, &m) != nil {
		return
	}
	now := time.Now()
	for k, v := range m {
		if t := time.Unix(v, 0); t.After(now) && len(k) == 64 {
			w.sessions[k] = t
		}
	}
}

func (w *Web) saveSessionsLocked() {
	if w.dataDir == "" {
		return
	}
	m := make(map[string]int64, len(w.sessions))
	now := time.Now()
	for k, t := range w.sessions {
		if t.After(now) {
			m[k] = t.Unix()
		}
	}
	b, _ := json.Marshal(m)
	_ = writeFileAtomic(w.sessionsPath(), b, 0o600)
	w.sessSaved = now
}

func (w *Web) startSession(rw http.ResponseWriter, r *http.Request) {
	tok := randomToken(32)
	w.smu.Lock()
	now := time.Now()
	for k, exp := range w.sessions {
		if now.After(exp) {
			delete(w.sessions, k)
		}
	}
	w.sessions[sessKey(tok)] = now.Add(sessionTTL)
	w.saveSessionsLocked()
	w.smu.Unlock()
	http.SetCookie(rw, &http.Cookie{
		Name: sessionCookie, Value: tok, Path: "/", HttpOnly: true,
		Secure: r.TLS != nil, SameSite: http.SameSiteStrictMode,
	})
}

func clientIP(r *http.Request) string {
	h, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return h
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func jsonOK(rw http.ResponseWriter, v any) {
	rw.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(rw).Encode(v)
}

func jsonErr(rw http.ResponseWriter, code int, msg string) {
	rw.Header().Set("Content-Type", "application/json; charset=utf-8")
	rw.WriteHeader(code)
	_ = json.NewEncoder(rw).Encode(map[string]string{"error": msg})
}

func (w *Web) fail(rw http.ResponseWriter, err error) {
	var ve *ValidationError
	switch {
	case errors.As(err, &ve):
		jsonErr(rw, http.StatusBadRequest, ve.Msg)
	case errors.Is(err, errNotFound):
		jsonErr(rw, http.StatusNotFound, "Kayıt bulunamadı")
	default:
		w.log.Printf("hata: %v", err)
		jsonErr(rw, http.StatusInternalServerError, "Beklenmeyen hata: "+err.Error())
	}
}

func decode(r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 8<<20))
	if err := dec.Decode(v); err != nil {
		return verr("İstek okunamadı: %v", err)
	}
	return nil
}

func validatePassword(p string) error {
	if len([]rune(p)) < 8 {
		return verr("Parola en az 8 karakter olmalı")
	}
	if len(p) > 72 {
		return verr("Parola en fazla 72 bayt olabilir")
	}
	return nil
}

// ---------------------------------------------------------------------------
// Auth handlers
// ---------------------------------------------------------------------------

func (w *Web) handleStatus(rw http.ResponseWriter, r *http.Request) {
	host, _ := os.Hostname()
	jsonOK(rw, map[string]any{
		"setupRequired": w.store.PasswordHash() == "",
		"authenticated": w.authed(r),
		"version":       w.version,
		"hostname":      host,
	})
}

func (w *Web) handleSetup(rw http.ResponseWriter, r *http.Request) {
	var in struct {
		Password string `json:"password"`
	}
	if err := decode(r, &in); err != nil {
		w.fail(rw, err)
		return
	}
	if w.store.PasswordHash() != "" {
		jsonErr(rw, http.StatusConflict, "Parola zaten belirlenmiş")
		return
	}
	if err := validatePassword(in.Password); err != nil {
		w.fail(rw, err)
		return
	}
	h, err := bcrypt.GenerateFromPassword([]byte(in.Password), 11)
	if err != nil {
		w.fail(rw, err)
		return
	}
	if err := w.store.SetPasswordHash(string(h)); err != nil {
		w.fail(rw, err)
		return
	}
	w.log.Printf("yönetici parolası belirlendi (%s)", clientIP(r))
	w.startSession(rw, r)
	jsonOK(rw, map[string]bool{"ok": true})
}

func (w *Web) handleLogin(rw http.ResponseWriter, r *http.Request) {
	var in struct {
		Password string `json:"password"`
	}
	if err := decode(r, &in); err != nil {
		w.fail(rw, err)
		return
	}
	ip := clientIP(r)
	now := time.Now()
	w.smu.Lock()
	f := w.fails[ip]
	if f != nil && now.Before(f.blockedUntil) {
		wait := time.Until(f.blockedUntil).Round(time.Minute)
		w.smu.Unlock()
		jsonErr(rw, http.StatusTooManyRequests, fmt.Sprintf("Çok fazla hatalı deneme. %d dakika sonra tekrar deneyin.", int(wait.Minutes())+1))
		return
	}
	w.smu.Unlock()
	hash := w.store.PasswordHash()
	if hash == "" || bcrypt.CompareHashAndPassword([]byte(hash), []byte(in.Password)) != nil {
		w.smu.Lock()
		f = w.fails[ip]
		if f == nil || now.Sub(f.first) > 15*time.Minute {
			f = &loginFail{first: now}
			w.fails[ip] = f
		}
		f.count++
		if f.count >= 5 {
			f.blockedUntil = now.Add(10 * time.Minute)
			f.count = 0
			f.first = now
		}
		w.smu.Unlock()
		w.log.Printf("hatalı giriş denemesi (%s)", ip)
		time.Sleep(700 * time.Millisecond)
		jsonErr(rw, http.StatusUnauthorized, "Parola hatalı")
		return
	}
	w.smu.Lock()
	delete(w.fails, ip)
	w.smu.Unlock()
	w.startSession(rw, r)
	jsonOK(rw, map[string]bool{"ok": true})
}

func (w *Web) handleLogout(rw http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		w.smu.Lock()
		delete(w.sessions, sessKey(c.Value))
		w.saveSessionsLocked()
		w.smu.Unlock()
	}
	http.SetCookie(rw, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteStrictMode})
	jsonOK(rw, map[string]bool{"ok": true})
}

func (w *Web) handlePassword(rw http.ResponseWriter, r *http.Request) {
	var in struct {
		Current  string `json:"current"`
		Password string `json:"password"`
	}
	if err := decode(r, &in); err != nil {
		w.fail(rw, err)
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(w.store.PasswordHash()), []byte(in.Current)) != nil {
		jsonErr(rw, http.StatusBadRequest, "Mevcut parola hatalı")
		return
	}
	if err := validatePassword(in.Password); err != nil {
		w.fail(rw, err)
		return
	}
	h, err := bcrypt.GenerateFromPassword([]byte(in.Password), 11)
	if err != nil {
		w.fail(rw, err)
		return
	}
	if err := w.store.SetPasswordHash(string(h)); err != nil {
		w.fail(rw, err)
		return
	}
	cur, _ := r.Cookie(sessionCookie)
	w.smu.Lock()
	for k := range w.sessions {
		if cur == nil || k != sessKey(cur.Value) {
			delete(w.sessions, k)
		}
	}
	w.saveSessionsLocked()
	w.smu.Unlock()
	jsonOK(rw, map[string]bool{"ok": true})
}
