package main

import (
	"bytes"
	"encoding/binary"
	"math"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func floatWAV(rate int, samples []float32, bogusSize bool) []byte {
	var b bytes.Buffer
	b.WriteString("RIFF")
	size := uint32(36 + len(samples)*4)
	if bogusSize {
		size = 0x7ffff024
	}
	binary.Write(&b, binary.LittleEndian, size)
	b.WriteString("WAVEfmt ")
	for _, v := range []any{uint32(16), uint16(3), uint16(1), uint32(rate), uint32(rate * 4), uint16(4), uint16(32)} {
		binary.Write(&b, binary.LittleEndian, v)
	}
	b.WriteString("data")
	ds := uint32(len(samples) * 4)
	if bogusSize {
		ds = 0x7ffff000
	}
	binary.Write(&b, binary.LittleEndian, ds)
	for _, s := range samples {
		binary.Write(&b, binary.LittleEndian, math.Float32bits(s))
	}
	return b.Bytes()
}

func TestReadWAVFloatStreaming(t *testing.T) {
	in := make([]float32, 22050)
	for i := range in {
		in[i] = float32(0.5 * math.Sin(2*math.Pi*440*float64(i)/22050))
	}
	p, err := ReadWAV(bytes.NewReader(floatWAV(22050, in, true)))
	if err != nil {
		t.Fatal(err)
	}
	if p.Rate != 22050 || len(p.Samples) != len(in) {
		t.Fatalf("rate %d len %d", p.Rate, len(p.Samples))
	}
	out, err := PrepareAnnouncement(p)
	if err != nil {
		t.Fatal(err)
	}
	if out.Rate != 8000 {
		t.Fatalf("rate %d", out.Rate)
	}
	if d := out.Duration(); d < 900*time.Millisecond || d > 1100*time.Millisecond {
		t.Fatalf("duration %v", d)
	}
	var peak float32
	for _, v := range out.Samples {
		if v > peak {
			peak = v
		}
	}
	if peak < 0.8 || peak > 0.9 { // normalized to -1.5 dBFS
		t.Fatalf("peak %v", peak)
	}
	again, err := ReadWAV(bytes.NewReader(EncodeWAV16(out)))
	if err != nil || len(again.Samples) != len(out.Samples) {
		t.Fatalf("round trip: %v %d", err, len(again.Samples))
	}
}

func TestPrepareAnnouncementRejectsSilence(t *testing.T) {
	if _, err := PrepareAnnouncement(Silence(2*time.Second, 16000)); err == nil {
		t.Fatal("silence accepted")
	}
}

func TestSlugAndRecordingPath(t *testing.T) {
	if s := slug("Türkiye hattı"); s != "Turkiye-hatti" {
		t.Fatalf("slug %q", s)
	}
	if s := slug("  ...  "); s != "hat" {
		t.Fatalf("slug %q", s)
	}
	at := time.Date(2026, 11, 19, 14, 21, 4, 0, time.UTC)
	got := recordingRelPath("line-date", at, "Destek", "+49 151 5550-4411", "mp3")
	if got != filepath.Join("Destek", "2026", "11", "19", "142104_+4915155504411.mp3") {
		t.Fatalf("path %q", got)
	}
	if got := recordingRelPath("flat", at, "Satış", "", "wav"); got != "2026-11-19_142104_Satis_gizli.wav" {
		t.Fatalf("path %q", got)
	}
}

func TestParseImport(t *testing.T) {
	rows := parseImport("ad;numara;sunucu;kullanıcı;şifre\nSatış 2;+49 221 1;sip.ex.de;u1;p1\nFuar,+49 2,sip.ex.de,u2,\n\n# not\nBayi\t+49 3\tsip.ex.de\tu3\tp3\tauth3", newLineDefaults())
	if len(rows) != 3 {
		t.Fatalf("rows %d: %+v", len(rows), rows)
	}
	if rows[0].Error != "" || rows[0].line.Name != "Satış 2" || rows[0].line.Password != "p1" {
		t.Fatalf("row0 %+v", rows[0])
	}
	if rows[1].Error == "" {
		t.Fatalf("missing password not reported: %+v", rows[1])
	}
	if rows[2].line.AuthUser != "auth3" || rows[2].Error != "" {
		t.Fatalf("row2 %+v", rows[2])
	}
}

func TestStoreValidation(t *testing.T) {
	s, err := OpenStore(filepath.Join(t.TempDir(), "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveExtension("", Extension{Num: "101", Name: "Ofis"}); err != nil {
		t.Fatal(err)
	}
	e, _ := s.Extension("101")
	if len(e.Password) != 16 {
		t.Fatalf("generated password %q", e.Password)
	}
	l := newLineDefaults()
	if l.Mode != "busy" || l.NoAnswer != "busy" || l.RingSec != 30 {
		t.Fatalf("new line defaults: %+v", l)
	}
	l.Mode = "ring"
	l.Name, l.Server, l.Username, l.Password, l.Exts = "Destek", "sip:Sip.Example.de:5080", "4930123", "a;b#c", []string{"101"}
	saved, err := s.SaveLine(l)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Server != "sip.example.de" || saved.Port != 5080 {
		t.Fatalf("server %q port %d", saved.Server, saved.Port)
	}
	l2 := l
	l2.ID = ""
	if _, err := s.SaveLine(l2); err == nil || !strings.Contains(err.Error(), "başka bir hat") {
		t.Fatalf("duplicate name accepted: %v", err)
	}
	if _, err := s.SetLineMode(saved.ID, "annring"); err == nil {
		t.Fatal("annring without announcement accepted")
	}
	if err := s.DeleteExtension("101"); err == nil {
		t.Fatal("deleting the only ringing extension accepted")
	}
	// empty password on update keeps the stored one
	saved.Password = ""
	upd, err := s.SaveLine(saved)
	if err != nil || upd.Password != "a;b#c" {
		t.Fatalf("password not kept: %v %q", err, upd.Password)
	}
}

func TestBusyModeAndBulkUpdate(t *testing.T) {
	s, err := OpenStore(filepath.Join(t.TempDir(), "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, name := range []string{"Hat 1", "Hat 2", "Hat 3"} {
		l := newLineDefaults() // "Çalıp meşgul": no extension needed
		l.Name, l.Server, l.Username, l.Password = name, "sip.ex.de", "u-"+name[4:], "pw"
		saved, err := s.SaveLine(l)
		if err != nil {
			t.Fatalf("busy line without extension rejected: %v", err)
		}
		ids = append(ids, saved.ID)
	}
	// switching to "ring" without extensions must fail for all and change nothing
	if _, err := s.BulkUpdate(ids, func(l *Line) { l.Mode = "ring" }); err == nil || !strings.Contains(err.Error(), "ve 2 hat daha") {
		t.Fatalf("bulk ring without extensions: %v", err)
	}
	for _, l := range s.Lines() {
		if l.Mode != "busy" {
			t.Fatalf("partial bulk update: %+v", l)
		}
	}
	if _, err := s.SaveExtension("", Extension{Num: "101", Name: "Ofis"}); err != nil {
		t.Fatal(err)
	}
	n, err := s.BulkUpdate(ids[:2], func(l *Line) { l.Mode, l.Exts, l.RingSec = "ring", []string{"101"}, 45 })
	if err != nil || n != 2 {
		t.Fatalf("bulk update: %d %v", n, err)
	}
	if n, _ := s.BulkUpdate(ids, func(l *Line) { l.Enabled = false }); n != 3 {
		t.Fatalf("bulk disable %d", n)
	}
	if n, err := s.DeleteLines(ids[1:]); err != nil || n != 2 || len(s.Lines()) != 1 {
		t.Fatalf("bulk delete %d %v", n, err)
	}
	l := s.Lines()[0]
	if l.Mode != "ring" || l.RingSec != 45 || l.Enabled {
		t.Fatalf("after bulk: %+v", l)
	}
	files := renderAsteriskConfig(Config{Lines: []Line{{ID: "b1", Name: "Meşgul", Enabled: true, Server: "x", Port: 5060, Username: "u", Password: "p",
		Transport: "udp", Codecs: []string{"alaw"}, DTMF: "rfc4733", Mode: "busy", RingSec: 30, Record: true}}, Settings: defaultSettings()}, astRuntime{})
	dp := files["extensions.conf"]
	for _, want := range []string{"Ringing()", "Wait(30)", "Busy(3)", "Hangup(17)"} {
		if !strings.Contains(dp, want) {
			t.Errorf("busy dialplan lacks %q", want)
		}
	}
	if strings.Contains(dp, "MixMonitor") || strings.Contains(dp, "Dial(") {
		t.Error("busy mode must neither record nor ring a phone")
	}
}

func TestRenderConfigEscapingAndFlow(t *testing.T) {
	cfg := Config{AMISecret: "sekret", Settings: defaultSettings()}
	cfg.Extensions = []Extension{{Num: "101", Name: `Ofis "Mac"; <x>`, Password: "pw12345678"}}
	cfg.Announcements = []Announcement{{ID: "a1", Name: "Kayıt"}}
	cfg.Lines = []Line{{ID: "l1", Name: "Destek (TR), 1", Enabled: true, Server: "sip.ex.de", Port: 5060, Username: "u1", Password: "p;w",
		Transport: "udp", Expiry: 300, Codecs: []string{"alaw"}, DTMF: "rfc4733", Mode: "annring", AnnID: "a1", Exts: []string{"101"},
		RingSec: 20, NoAnswer: "vm", VMMaxSec: 90, Record: true, CIDPrefix: true}}
	files := renderAsteriskConfig(cfg, astRuntime{LocalNets: []string{"192.168.1.0/24"}, ExtIP: "203.0.113.5", Blocked: []string{"198.51.100.7"}})
	pj := files["pjsip.conf"]
	for _, want := range []string{`password = p\;w`, "line = yes", "endpoint = line-l1", "external_media_address = 203.0.113.5",
		"deny = 198.51.100.7/255.255.255.255", `callerid = "Ofis Mac x" <101>`, "permit = 192.168.0.0/255.255.0.0"} {
		if !strings.Contains(pj, want) {
			t.Errorf("pjsip.conf lacks %q", want)
		}
	}
	dp := files["extensions.conf"]
	for _, want := range []string{"[sl-l1]", "Goto(sl-l1-in,s,1)", "Playback(santral/a1)", "Dial(PJSIP/101,20,rU(santral-answered^${UNIQUEID}))",
		"Gosub(santral-vm,s,1(90))", "Set(CALLERID(name)=[Destek TR 1] ${SL_DN})", "MixMonitor(${SL_REC}.wav"} {
		if !strings.Contains(dp, want) {
			t.Errorf("extensions.conf lacks %q", want)
		}
	}
	if strings.Contains(files["asterisk.conf"], "(!)") {
		t.Error("asterisk.conf directories must not be a template")
	}
}

func TestParseSTUN(t *testing.T) {
	txid := []byte("abcdefghijkl")
	var b bytes.Buffer
	binary.Write(&b, binary.BigEndian, uint16(0x0101))
	binary.Write(&b, binary.BigEndian, uint16(12))
	binary.Write(&b, binary.BigEndian, uint32(0x2112A442))
	b.Write(txid)
	binary.Write(&b, binary.BigEndian, uint16(0x0020))
	binary.Write(&b, binary.BigEndian, uint16(8))
	b.Write([]byte{0, 1, 0, 0, 203 ^ 0x21, 0 ^ 0x12, 113 ^ 0xA4, 9 ^ 0x42})
	if ip := parseSTUN(b.Bytes(), txid); ip != "203.0.113.9" {
		t.Fatalf("ip %q", ip)
	}
}

func TestCallLogQueryAndPrune(t *testing.T) {
	c, err := OpenCallLog(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	_ = c.Add(CallRecord{ID: "1", Start: now.Add(-48 * time.Hour), LineID: "l1", Caller: "+491701", Result: "answered", Rec: "a.mp3", RecSec: 10})
	_ = c.Add(CallRecord{ID: "2", Start: now.Add(-time.Hour), LineID: "l1", Caller: "+491702", Result: "missed"})
	_ = c.Add(CallRecord{ID: "3", Start: now, LineID: "l2", Caller: "+90532", Result: "voicemail", Rec: "b.mp3", RecSec: 5})
	items, total, sum := c.Query(CallQuery{From: now.Add(-2 * time.Hour)})
	if total != 2 || items[0].ID != "3" || sum.Voicemail != 1 || sum.Missed != 1 || sum.Files != 1 {
		t.Fatalf("query: %d %+v %+v", total, items, sum)
	}
	if _, n, _ := c.Query(CallQuery{Search: "90 532"}); n != 1 {
		t.Fatalf("search by digits: %d", n)
	}
	var deleted []string
	if n := c.Prune(now.Add(-24*time.Hour), func(r CallRecord) { deleted = append(deleted, r.Rec) }); n != 1 || len(deleted) != 1 {
		t.Fatalf("prune %d %v", n, deleted)
	}
	c2, _ := OpenCallLog(c.dir)
	if _, n, _ := c2.Query(CallQuery{}); n != 2 {
		t.Fatalf("reloaded %d", n)
	}
}
