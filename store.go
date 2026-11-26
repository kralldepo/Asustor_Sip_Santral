package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// ---------------------------------------------------------------------------
// Model
// ---------------------------------------------------------------------------

// Line is one SIP account (inbound number) registered at a provider.
type Line struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Number        string   `json:"number"`
	Enabled       bool     `json:"enabled"`
	Server        string   `json:"server"`
	Port          int      `json:"port"`
	Username      string   `json:"username"`
	AuthUser      string   `json:"authUser"`
	Password      string   `json:"password"`
	Transport     string   `json:"transport"` // udp | tcp
	Expiry        int      `json:"expiry"`
	OutboundProxy string   `json:"outboundProxy"`
	FromDomain    string   `json:"fromDomain"`
	Codecs        []string `json:"codecs"`
	DTMF          string   `json:"dtmf"` // rfc4733 | info | inband

	Mode       string   `json:"mode"` // ring | annring | ann | busy
	AnnID      string   `json:"annId"`
	Exts       []string `json:"exts"`
	RingSec    int      `json:"ringSec"`
	NoAnswer   string   `json:"noAnswer"` // hang | busy | ann | vm
	NoAnsAnnID string   `json:"noAnsAnnId"`
	After      string   `json:"after"` // ann mode: hang | vm
	VMMaxSec   int      `json:"vmMaxSec"`
	Record     bool     `json:"record"`
	CIDPrefix  bool     `json:"cidPrefix"`

	Created time.Time `json:"created"`
}

// Extension is a phone (softphone, desk phone) that registers to the NAS.
type Extension struct {
	Num      string    `json:"num"`
	Name     string    `json:"name"`
	Password string    `json:"password"`
	Created  time.Time `json:"created"`
}

// Announcement is a prompt played to callers (8 kHz mono WAV on disk).
type Announcement struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Source   string    `json:"source"` // file | mic | tts
	FileName string    `json:"fileName,omitempty"`
	Text     string    `json:"text,omitempty"`
	Voice    string    `json:"voice,omitempty"`
	Speed    float64   `json:"speed,omitempty"`
	DurMs    int       `json:"durMs"`
	Created  time.Time `json:"created"`
}

type Settings struct {
	ShareFolder   string   `json:"shareFolder"`
	FolderLayout  string   `json:"folderLayout"` // line-date | date-line | flat
	Format        string   `json:"format"`       // mp3 | wav
	RetentionDays int      `json:"retentionDays"`
	SIPPort       int      `json:"sipPort"`
	UDP           bool     `json:"udp"`
	TCP           bool     `json:"tcp"`
	RTPStart      int      `json:"rtpStart"`
	RTPEnd        int      `json:"rtpEnd"`
	ExtIPMode     string   `json:"extIpMode"` // auto | manual | off
	ExtIP         string   `json:"extIp"`
	LocalNetMode  string   `json:"localNetMode"` // auto | manual
	LocalNets     []string `json:"localNets"`
	ExtLANOnly    bool     `json:"extLanOnly"`
	BlockFailed   bool     `json:"blockFailed"`
	Timezone      string   `json:"timezone"`
	SIPDebug      bool     `json:"sipDebug"`
	ToneZone      string   `json:"toneZone"` // de | tr | ...
}

type Config struct {
	Version       int            `json:"version"`
	PasswordHash  string         `json:"passwordHash"`
	AMISecret     string         `json:"amiSecret"`
	Lines         []Line         `json:"lines"`
	Extensions    []Extension    `json:"extensions"`
	Announcements []Announcement `json:"announcements"`
	Settings      Settings       `json:"settings"`
}

// Location returns the configured time zone (falls back to system local).
func (s Settings) Location() *time.Location {
	if s.Timezone != "" {
		if loc, err := time.LoadLocation(s.Timezone); err == nil {
			return loc
		}
	}
	return time.Local
}

// ---------------------------------------------------------------------------
// Errors
// ---------------------------------------------------------------------------

type ValidationError struct{ Msg string }

func (e *ValidationError) Error() string { return e.Msg }

func verr(format string, a ...any) error { return &ValidationError{Msg: fmt.Sprintf(format, a...)} }

var errNotFound = errors.New("bulunamadı")

// ---------------------------------------------------------------------------
// Defaults
// ---------------------------------------------------------------------------

func defaultSettings() Settings {
	return Settings{
		FolderLayout:  "line-date",
		Format:        "mp3",
		RetentionDays: 365,
		SIPPort:       5060,
		UDP:           true,
		TCP:           true,
		RTPStart:      10000,
		RTPEnd:        10200,
		ExtIPMode:     "auto",
		LocalNetMode:  "auto",
		ExtLANOnly:    true,
		BlockFailed:   true,
		ToneZone:      "de",
	}
}

func newLineDefaults() Line {
	return Line{
		Enabled: true, Port: 5060, Transport: "udp", Expiry: 300,
		Codecs: []string{"g722", "alaw", "ulaw"}, DTMF: "rfc4733",
		Mode: "busy", RingSec: 30, NoAnswer: "busy", After: "hang", VMMaxSec: 120,
		Record: true, CIDPrefix: true, Exts: []string{},
	}
}

func (c *Config) applyDefaults() {
	d := defaultSettings()
	s := &c.Settings
	if s.FolderLayout == "" {
		s.FolderLayout = d.FolderLayout
	}
	if s.Format == "" {
		s.Format = d.Format
	}
	if s.SIPPort == 0 {
		s.SIPPort = d.SIPPort
	}
	if !s.UDP && !s.TCP {
		s.UDP, s.TCP = true, true
	}
	if s.RTPStart == 0 || s.RTPEnd == 0 {
		s.RTPStart, s.RTPEnd = d.RTPStart, d.RTPEnd
	}
	if s.ExtIPMode == "" {
		s.ExtIPMode = d.ExtIPMode
	}
	if s.LocalNetMode == "" {
		s.LocalNetMode = d.LocalNetMode
	}
	if s.ToneZone == "" {
		s.ToneZone = d.ToneZone
	}
	if c.Lines == nil {
		c.Lines = []Line{}
	}
	if c.Extensions == nil {
		c.Extensions = []Extension{}
	}
	if c.Announcements == nil {
		c.Announcements = []Announcement{}
	}
	for i := range c.Lines {
		if c.Lines[i].Exts == nil {
			c.Lines[i].Exts = []string{}
		}
		if len(c.Lines[i].Codecs) == 0 {
			c.Lines[i].Codecs = []string{"g722", "alaw", "ulaw"}
		}
	}
}

// ---------------------------------------------------------------------------
// Store
// ---------------------------------------------------------------------------

type Store struct {
	mu        sync.RWMutex
	path      string
	cfg       Config
	listeners []func()
}

func OpenStore(path string) (*Store, error) {
	s := &Store{path: path}
	b, err := os.ReadFile(path)
	switch {
	case err == nil:
		if err := json.Unmarshal(b, &s.cfg); err != nil {
			return nil, fmt.Errorf("%s bozuk: %w", path, err)
		}
	case os.IsNotExist(err):
		s.cfg = Config{Version: 1, Settings: defaultSettings()}
	default:
		return nil, err
	}
	s.cfg.applyDefaults()
	if s.cfg.AMISecret == "" {
		s.cfg.AMISecret = randomToken(16)
	}
	if err := s.save(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) save() error {
	b, err := json.MarshalIndent(s.cfg, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(s.path, b, 0o600)
}

func (s *Store) OnChange(f func()) {
	s.mu.Lock()
	s.listeners = append(s.listeners, f)
	s.mu.Unlock()
}

// commit saves and notifies listeners; caller holds the write lock.
func (s *Store) commit() error {
	if err := s.save(); err != nil {
		return err
	}
	ls := append([]func(){}, s.listeners...)
	go func() {
		for _, f := range ls {
			f()
		}
	}()
	return nil
}

func clone[T any](v T) T {
	b, _ := json.Marshal(v)
	var out T
	_ = json.Unmarshal(b, &out)
	return out
}

func (s *Store) Snapshot() Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return clone(s.cfg)
}

func (s *Store) Settings() Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return clone(s.cfg.Settings)
}

func (s *Store) PasswordHash() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg.PasswordHash
}

func (s *Store) SetPasswordHash(h string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cfg.PasswordHash = h
	return s.save()
}

func (s *Store) AMISecret() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg.AMISecret
}

// ---------------------------------------------------------------------------
// Validation helpers
// ---------------------------------------------------------------------------

var (
	reSIPUser = regexp.MustCompile(`^[A-Za-z0-9._+\-*!~]{1,64}$`)
	reHost    = regexp.MustCompile(`^[A-Za-z0-9.\-]{1,253}$`)
	reExtNum  = regexp.MustCompile(`^[0-9]{2,6}$`)
	reNumber  = regexp.MustCompile(`^[0-9+ ()\-/]{0,32}$`)
)

func runeLen(s string) int { return utf8.RuneCountInString(s) }

func cleanName(s string, max int, what string) (string, error) {
	s = strings.Join(strings.Fields(s), " ")
	if s == "" {
		return "", verr("%s boş olamaz", what)
	}
	if runeLen(s) > max {
		return "", verr("%s en fazla %d karakter olabilir", what, max)
	}
	if strings.ContainsAny(s, "\x00\r\n") {
		return "", verr("%s geçersiz karakter içeriyor", what)
	}
	return s, nil
}

// splitHostPort accepts "host", "host:port", "sip:host:port", "[v6]:port".
func splitHostPort(in string, defPort int) (string, int, error) {
	s := strings.TrimSpace(in)
	s = strings.TrimPrefix(strings.TrimPrefix(s, "sip:"), "sips:")
	s = strings.TrimSuffix(s, "/")
	if s == "" {
		return "", 0, verr("Sunucu adresi boş olamaz")
	}
	host, port := s, defPort
	if h, p, err := net.SplitHostPort(s); err == nil {
		n, err := strconv.Atoi(p)
		if err != nil || n < 1 || n > 65535 {
			return "", 0, verr("Port geçersiz: %s", p)
		}
		host, port = h, n
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.String(), port, nil
	}
	if !reHost.MatchString(host) || strings.HasPrefix(host, "-") || strings.Contains(host, "..") {
		return "", 0, verr("Sunucu adresi geçersiz: %s", in)
	}
	return strings.ToLower(host), port, nil
}

func validPassword(p, what string) error {
	if p == "" {
		return verr("%s boş olamaz", what)
	}
	if len(p) > 128 {
		return verr("%s çok uzun", what)
	}
	if strings.ContainsAny(p, "\x00\r\n\t") || strings.TrimSpace(p) != p {
		return verr("%s başında/sonunda boşluk ya da satır sonu olamaz", what)
	}
	return nil
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func (s *Store) annExistsLocked(id string) bool {
	for _, a := range s.cfg.Announcements {
		if a.ID == id {
			return true
		}
	}
	return false
}

func (s *Store) extExistsLocked(num string) bool {
	for _, e := range s.cfg.Extensions {
		if e.Num == num {
			return true
		}
	}
	return false
}

// normalizeLine validates a line against the current config (lock held).
func (s *Store) normalizeLineLocked(in Line) (Line, error) {
	var err error
	if in.Name, err = cleanName(in.Name, 40, "Hat adı"); err != nil {
		return in, err
	}
	for _, l := range s.cfg.Lines {
		if l.ID != in.ID && strings.EqualFold(l.Name, in.Name) {
			return in, verr("“%s” adında başka bir hat var", in.Name)
		}
	}
	in.Number = strings.TrimSpace(in.Number)
	if !reNumber.MatchString(in.Number) {
		return in, verr("Numara yalnız rakam, boşluk, + ( ) - / içerebilir")
	}
	if in.Port == 0 {
		in.Port = 5060
	}
	if in.Server, in.Port, err = splitHostPort(in.Server, in.Port); err != nil {
		return in, err
	}
	in.Username = strings.TrimSpace(in.Username)
	if !reSIPUser.MatchString(in.Username) {
		return in, verr("Kullanıcı adı geçersiz (harf, rakam ve . _ + - * ! ~ olabilir)")
	}
	in.AuthUser = strings.TrimSpace(in.AuthUser)
	if in.AuthUser != "" && !reSIPUser.MatchString(in.AuthUser) {
		return in, verr("Kimlik kullanıcısı geçersiz")
	}
	if err := validPassword(in.Password, "Şifre"); err != nil {
		return in, err
	}
	if in.Transport == "" {
		in.Transport = "udp"
	}
	if in.Transport != "udp" && in.Transport != "tcp" {
		return in, verr("Taşıma UDP ya da TCP olmalı")
	}
	if in.Expiry == 0 {
		in.Expiry = 300
	}
	if in.Expiry < 60 || in.Expiry > 3600 {
		return in, verr("Kayıt yenileme 60–3600 saniye olmalı")
	}
	in.OutboundProxy = strings.TrimSpace(in.OutboundProxy)
	if in.OutboundProxy != "" {
		h, p, err := splitHostPort(in.OutboundProxy, 5060)
		if err != nil {
			return in, verr("Outbound proxy geçersiz")
		}
		in.OutboundProxy = net.JoinHostPort(h, strconv.Itoa(p))
	}
	in.FromDomain = strings.TrimSpace(in.FromDomain)
	if in.FromDomain != "" {
		h, _, err := splitHostPort(in.FromDomain, 0)
		if err != nil {
			return in, verr("From domain geçersiz")
		}
		in.FromDomain = h
	}
	var codecs []string
	for _, c := range in.Codecs {
		if (c == "g722" || c == "alaw" || c == "ulaw") && !contains(codecs, c) {
			codecs = append(codecs, c)
		}
	}
	if len(codecs) == 0 {
		codecs = []string{"g722", "alaw", "ulaw"}
	}
	in.Codecs = codecs
	if in.DTMF == "" {
		in.DTMF = "rfc4733"
	}
	if in.DTMF != "rfc4733" && in.DTMF != "info" && in.DTMF != "inband" {
		return in, verr("DTMF türü geçersiz")
	}

	switch in.Mode {
	case "ring", "annring", "ann", "busy":
	default:
		return in, verr("Gelen arama modu geçersiz")
	}
	if in.Mode == "annring" || in.Mode == "ann" {
		if in.AnnID == "" || !s.annExistsLocked(in.AnnID) {
			return in, verr("Bu mod için bir anons seçin")
		}
	} else if in.AnnID != "" && !s.annExistsLocked(in.AnnID) {
		in.AnnID = ""
	}
	exts := []string{}
	for _, e := range in.Exts {
		e = strings.TrimSpace(e)
		if e == "" || contains(exts, e) {
			continue
		}
		if !s.extExistsLocked(e) {
			return in, verr("Dahili %s yok", e)
		}
		exts = append(exts, e)
	}
	in.Exts = exts
	if (in.Mode == "ring" || in.Mode == "annring") && len(in.Exts) == 0 {
		return in, verr("Çalacak en az bir dahili seçin (dahilisiz çalıp meşgule düşmesi için “Çalıp meşgul” modunu seçin)")
	}
	if in.RingSec == 0 {
		in.RingSec = 30
	}
	if in.RingSec < 5 || in.RingSec > 300 {
		return in, verr("Çalma süresi 5–300 saniye olmalı")
	}
	if in.NoAnswer == "" {
		in.NoAnswer = "hang"
	}
	switch in.NoAnswer {
	case "hang", "vm", "busy":
	case "ann":
		if in.NoAnsAnnID == "" {
			return in, verr("Kimse açmazsa çalınacak anonsu seçin")
		}
	default:
		return in, verr("Cevapsız arama kuralı geçersiz")
	}
	if in.NoAnsAnnID != "" && !s.annExistsLocked(in.NoAnsAnnID) {
		return in, verr("Cevapsız arama anonsu bulunamadı")
	}
	if in.After == "" {
		in.After = "hang"
	}
	if in.After != "hang" && in.After != "vm" {
		return in, verr("Anons sonrası işlem geçersiz")
	}
	if in.VMMaxSec == 0 {
		in.VMMaxSec = 120
	}
	if in.VMMaxSec < 10 || in.VMMaxSec > 600 {
		return in, verr("Sesli mesaj süresi 10–600 saniye olmalı")
	}
	return in, nil
}

// ---------------------------------------------------------------------------
// Lines
// ---------------------------------------------------------------------------

func (s *Store) Lines() []Line {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return clone(s.cfg.Lines)
}

func (s *Store) Line(id string) (Line, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, l := range s.cfg.Lines {
		if l.ID == id {
			return clone(l), nil
		}
	}
	return Line{}, errNotFound
}

// ValidateLine checks a line without saving it.
func (s *Store) ValidateLine(in Line) (Line, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.normalizeLineLocked(in)
}

func (s *Store) SaveLine(in Line) (Line, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx := -1
	if in.ID != "" {
		for i, l := range s.cfg.Lines {
			if l.ID == in.ID {
				idx = i
				break
			}
		}
		if idx < 0 {
			return Line{}, errNotFound
		}
		if in.Password == "" { // keep the stored password when the form leaves it empty
			in.Password = s.cfg.Lines[idx].Password
		}
		in.Created = s.cfg.Lines[idx].Created
	}
	out, err := s.normalizeLineLocked(in)
	if err != nil {
		return Line{}, err
	}
	if idx < 0 {
		out.ID = newID("l")
		out.Created = time.Now()
		s.cfg.Lines = append(s.cfg.Lines, out)
	} else {
		s.cfg.Lines[idx] = out
	}
	return clone(out), s.commit()
}

// ImportLines validates and adds several lines at once (all or nothing).
func (s *Store) ImportLines(in []Line) ([]Line, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	saved := clone(s.cfg.Lines)
	var out []Line
	for i, l := range in {
		l.ID = ""
		n, err := s.normalizeLineLocked(l)
		if err != nil {
			s.cfg.Lines = saved
			return nil, verr("%d. hat (%s): %s", i+1, l.Name, err.Error())
		}
		n.ID = newID("l")
		n.Created = time.Now()
		s.cfg.Lines = append(s.cfg.Lines, n)
		out = append(out, n)
	}
	return clone(out), s.commit()
}

func (s *Store) DeleteLine(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, l := range s.cfg.Lines {
		if l.ID == id {
			s.cfg.Lines = append(s.cfg.Lines[:i], s.cfg.Lines[i+1:]...)
			return s.commit()
		}
	}
	return errNotFound
}

func (s *Store) SetLineMode(id, mode string) (Line, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, l := range s.cfg.Lines {
		if l.ID != id {
			continue
		}
		l.Mode = mode
		n, err := s.normalizeLineLocked(l)
		if err != nil {
			return Line{}, err
		}
		s.cfg.Lines[i] = n
		return clone(n), s.commit()
	}
	return Line{}, errNotFound
}

func (s *Store) SetLineEnabled(id string, on bool) (Line, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, l := range s.cfg.Lines {
		if l.ID == id {
			s.cfg.Lines[i].Enabled = on
			return clone(s.cfg.Lines[i]), s.commit()
		}
	}
	return Line{}, errNotFound
}

// BulkUpdate applies change to every listed line. Either all lines pass the
// validation and are saved together, or nothing changes.
func (s *Store) BulkUpdate(ids []string, change func(*Line)) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	want := map[string]bool{}
	for _, id := range ids {
		want[id] = true
	}
	next := clone(s.cfg.Lines)
	n := 0
	var errs []string
	for i := range next {
		if !want[next[i].ID] {
			continue
		}
		l := next[i]
		change(&l)
		out, err := s.normalizeLineLocked(l)
		if err != nil {
			errs = append(errs, "“"+next[i].Name+"”: "+err.Error())
			continue
		}
		next[i] = out
		n++
	}
	if len(errs) > 0 {
		msg := errs[0]
		if len(errs) > 1 {
			msg += fmt.Sprintf(" (ve %d hat daha)", len(errs)-1)
		}
		return 0, verr("%s", msg)
	}
	if n == 0 {
		return 0, verr("Seçili hat bulunamadı")
	}
	s.cfg.Lines = next
	return n, s.commit()
}

// DeleteLines removes the listed lines.
func (s *Store) DeleteLines(ids []string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	want := map[string]bool{}
	for _, id := range ids {
		want[id] = true
	}
	var keep []Line
	for _, l := range s.cfg.Lines {
		if !want[l.ID] {
			keep = append(keep, l)
		}
	}
	n := len(s.cfg.Lines) - len(keep)
	if n == 0 {
		return 0, verr("Seçili hat bulunamadı")
	}
	if keep == nil {
		keep = []Line{}
	}
	s.cfg.Lines = keep
	return n, s.commit()
}

// ---------------------------------------------------------------------------
// Extensions
// ---------------------------------------------------------------------------

func (s *Store) Extensions() []Extension {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := clone(s.cfg.Extensions)
	sort.Slice(out, func(i, j int) bool { return out[i].Num < out[j].Num })
	return out
}

func (s *Store) Extension(num string) (Extension, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, e := range s.cfg.Extensions {
		if e.Num == num {
			return e, nil
		}
	}
	return Extension{}, errNotFound
}

// SaveExtension creates (orig == "") or updates the extension orig.
func (s *Store) SaveExtension(orig string, in Extension) (Extension, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var err error
	in.Num = strings.TrimSpace(in.Num)
	if !reExtNum.MatchString(in.Num) {
		return Extension{}, verr("Dahili numarası 2–6 haneli olmalı")
	}
	if in.Name, err = cleanName(in.Name, 40, "Dahili adı"); err != nil {
		return Extension{}, err
	}
	in.Password = strings.TrimSpace(in.Password)
	idx := -1
	for i, e := range s.cfg.Extensions {
		if orig != "" && e.Num == orig {
			idx = i
		} else if e.Num == in.Num {
			return Extension{}, verr("%s numaralı dahili zaten var", in.Num)
		}
	}
	if orig != "" && idx < 0 {
		return Extension{}, errNotFound
	}
	if in.Password == "" {
		if idx >= 0 {
			in.Password = s.cfg.Extensions[idx].Password
		} else {
			in.Password = randomPassword(16)
		}
	}
	if len(in.Password) < 8 || len(in.Password) > 64 || strings.ContainsAny(in.Password, " \t\r\n;") {
		return Extension{}, verr("Dahili şifresi 8–64 karakter olmalı, boşluk ve ; içermemeli")
	}
	if idx < 0 {
		in.Created = time.Now()
		s.cfg.Extensions = append(s.cfg.Extensions, in)
	} else {
		in.Created = s.cfg.Extensions[idx].Created
		s.cfg.Extensions[idx] = in
		if orig != in.Num { // renumbered: follow it in the lines
			for i := range s.cfg.Lines {
				for j, e := range s.cfg.Lines[i].Exts {
					if e == orig {
						s.cfg.Lines[i].Exts[j] = in.Num
					}
				}
			}
		}
	}
	return in, s.commit()
}

func (s *Store) DeleteExtension(num string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var users []string
	for _, l := range s.cfg.Lines {
		if contains(l.Exts, num) && (l.Mode == "ring" || l.Mode == "annring") && len(l.Exts) == 1 {
			users = append(users, l.Name)
		}
	}
	if len(users) > 0 {
		return verr("Bu dahili şu hatlarda tek çalan dahili: %s. Önce o hatlara başka dahili ekleyin.", strings.Join(users, ", "))
	}
	for i, e := range s.cfg.Extensions {
		if e.Num == num {
			s.cfg.Extensions = append(s.cfg.Extensions[:i], s.cfg.Extensions[i+1:]...)
			for k := range s.cfg.Lines {
				var keep []string
				for _, x := range s.cfg.Lines[k].Exts {
					if x != num {
						keep = append(keep, x)
					}
				}
				if keep == nil {
					keep = []string{}
				}
				s.cfg.Lines[k].Exts = keep
			}
			return s.commit()
		}
	}
	return errNotFound
}

func (s *Store) ResetExtPassword(num string) (Extension, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, e := range s.cfg.Extensions {
		if e.Num == num {
			s.cfg.Extensions[i].Password = randomPassword(16)
			return s.cfg.Extensions[i], s.commit()
		}
	}
	return Extension{}, errNotFound
}

// ---------------------------------------------------------------------------
// Announcements
// ---------------------------------------------------------------------------

func (s *Store) Announcements() []Announcement {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return clone(s.cfg.Announcements)
}

func (s *Store) Announcement(id string) (Announcement, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, a := range s.cfg.Announcements {
		if a.ID == id {
			return a, nil
		}
	}
	return Announcement{}, errNotFound
}

func (s *Store) AddAnnouncement(a Announcement) (Announcement, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var err error
	if a.Name, err = cleanName(a.Name, 60, "Anons adı"); err != nil {
		return Announcement{}, err
	}
	for _, x := range s.cfg.Announcements {
		if strings.EqualFold(x.Name, a.Name) {
			return Announcement{}, verr("“%s” adında bir anons zaten var", a.Name)
		}
	}
	if a.ID == "" {
		a.ID = newID("a")
	}
	a.Created = time.Now()
	s.cfg.Announcements = append(s.cfg.Announcements, a)
	return a, s.commit()
}

func (s *Store) RenameAnnouncement(id, name string) (Announcement, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	name, err := cleanName(name, 60, "Anons adı")
	if err != nil {
		return Announcement{}, err
	}
	for _, x := range s.cfg.Announcements {
		if x.ID != id && strings.EqualFold(x.Name, name) {
			return Announcement{}, verr("“%s” adında bir anons zaten var", name)
		}
	}
	for i, a := range s.cfg.Announcements {
		if a.ID == id {
			s.cfg.Announcements[i].Name = name
			return s.cfg.Announcements[i], s.commit()
		}
	}
	return Announcement{}, errNotFound
}

// AnnouncementUsers lists the lines that play the announcement.
func (s *Store) AnnouncementUsers(id string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []string
	for _, l := range s.cfg.Lines {
		if l.AnnID == id || l.NoAnsAnnID == id {
			out = append(out, l.Name)
		}
	}
	return out
}

func (s *Store) DeleteAnnouncement(id string) error {
	if users := s.AnnouncementUsers(id); len(users) > 0 {
		return verr("Bu anons şu hatlarda kullanılıyor: %s", strings.Join(users, ", "))
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, a := range s.cfg.Announcements {
		if a.ID == id {
			s.cfg.Announcements = append(s.cfg.Announcements[:i], s.cfg.Announcements[i+1:]...)
			return s.commit()
		}
	}
	return errNotFound
}

// ---------------------------------------------------------------------------
// Settings
// ---------------------------------------------------------------------------

func normalizeCIDR(s string) (string, error) {
	s = strings.TrimSpace(s)
	if !strings.Contains(s, "/") {
		s += "/32"
	}
	_, n, err := net.ParseCIDR(s)
	if err != nil || n.IP.To4() == nil {
		return "", verr("Ağ adresi geçersiz: %s (ör. 192.168.1.0/24)", s)
	}
	return n.String(), nil
}

func (s *Store) SaveSettings(in Settings) (Settings, error) {
	in.ShareFolder = strings.TrimSpace(in.ShareFolder)
	switch in.FolderLayout {
	case "line-date", "date-line", "flat":
	default:
		return in, verr("Klasör düzeni geçersiz")
	}
	if in.Format != "mp3" && in.Format != "wav" {
		return in, verr("Kayıt biçimi MP3 ya da WAV olmalı")
	}
	if in.RetentionDays < 0 || in.RetentionDays > 3650 {
		return in, verr("Saklama süresi 0–3650 gün olmalı")
	}
	if in.SIPPort < 1024 || in.SIPPort > 65535 {
		return in, verr("SIP portu 1024–65535 olmalı")
	}
	if !in.UDP && !in.TCP {
		return in, verr("En az bir taşıma (UDP/TCP) açık olmalı")
	}
	if in.RTPStart < 1024 || in.RTPEnd > 65535 || in.RTPEnd-in.RTPStart < 20 {
		return in, verr("RTP aralığı geçersiz (en az 20 port, 1024–65535)")
	}
	if in.SIPPort >= in.RTPStart && in.SIPPort <= in.RTPEnd {
		return in, verr("SIP portu RTP aralığının içinde olamaz")
	}
	if in.SIPPort == 9490 || in.SIPPort == 5038 {
		return in, verr("Bu port panel/AMI tarafından kullanılıyor")
	}
	switch in.ExtIPMode {
	case "auto", "off":
	case "manual":
		ip := net.ParseIP(strings.TrimSpace(in.ExtIP))
		if ip == nil || ip.To4() == nil {
			return in, verr("Dış IP adresi geçersiz")
		}
		in.ExtIP = ip.String()
	default:
		return in, verr("Dış IP ayarı geçersiz")
	}
	switch in.LocalNetMode {
	case "auto":
	case "manual":
		var nets []string
		for _, n := range in.LocalNets {
			if strings.TrimSpace(n) == "" {
				continue
			}
			c, err := normalizeCIDR(n)
			if err != nil {
				return in, err
			}
			nets = append(nets, c)
		}
		if len(nets) == 0 {
			return in, verr("En az bir yerel ağ girin")
		}
		in.LocalNets = nets
	default:
		return in, verr("Yerel ağ ayarı geçersiz")
	}
	in.Timezone = strings.TrimSpace(in.Timezone)
	if in.Timezone != "" {
		if _, err := time.LoadLocation(in.Timezone); err != nil {
			return in, verr("Saat dilimi tanınmadı: %s", in.Timezone)
		}
	}
	if in.ToneZone == "" {
		in.ToneZone = "de"
	}
	if _, ok := toneZones[in.ToneZone]; !ok {
		return in, verr("Ton bölgesi geçersiz")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cfg.Settings = in
	return clone(in), s.commit()
}

// SetSIPDebug flips the verbose SIP log flag without other validation.
func (s *Store) SetSIPDebug(on bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cfg.Settings.SIPDebug = on
	return s.save()
}

// Restore replaces lines, extensions, announcements and settings from a
// backup. Password hash and AMI secret are kept.
func (s *Store) Restore(in Config) error {
	in.applyDefaults()
	s.mu.Lock()
	defer s.mu.Unlock()
	old := clone(s.cfg)
	s.cfg.Announcements = in.Announcements
	s.cfg.Extensions = nil
	for _, e := range in.Extensions {
		if !reExtNum.MatchString(e.Num) || e.Password == "" {
			s.cfg = old
			return verr("Yedekte geçersiz dahili: %s", e.Num)
		}
		s.cfg.Extensions = append(s.cfg.Extensions, e)
	}
	s.cfg.Lines = nil
	for _, l := range in.Lines {
		n, err := s.normalizeLineLocked(l)
		if err != nil {
			s.cfg = old
			return verr("Yedekteki “%s” hattı: %s", l.Name, err.Error())
		}
		s.cfg.Lines = append(s.cfg.Lines, n)
	}
	s.cfg.Settings = in.Settings
	s.cfg.applyDefaults()
	return s.commit()
}
