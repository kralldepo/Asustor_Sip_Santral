package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

func (w *Web) loc() *time.Location { return w.store.Settings().Location() }

func (w *Web) dayStart(t time.Time) time.Time {
	t = t.In(w.loc())
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// ---------------------------------------------------------------------------
// DTOs
// ---------------------------------------------------------------------------

type lineDTO struct {
	Line
	HasPassword  bool       `json:"hasPassword"`
	Status       LineStatus `json:"status"`
	Today        int        `json:"today"`
	AnnName      string     `json:"annName,omitempty"`
	NoAnsAnnName string     `json:"noAnsAnnName,omitempty"`
}

func (w *Web) lineDTOs(lines []Line) []lineDTO {
	anns := map[string]string{}
	for _, a := range w.store.Announcements() {
		anns[a.ID] = a.Name
	}
	today := w.calls.CountByLine(w.dayStart(time.Now()))
	out := make([]lineDTO, 0, len(lines))
	for _, l := range lines {
		d := lineDTO{Line: l, HasPassword: l.Password != "", Status: w.pbx.LineStatusOf(l.ID), Today: today[l.ID],
			AnnName: anns[l.AnnID], NoAnsAnnName: anns[l.NoAnsAnnID]}
		if !l.Enabled {
			d.Status = LineStatus{State: "disabled"}
		}
		d.Password = ""
		out = append(out, d)
	}
	return out
}

type liveDTO struct {
	LiveCall
	LineName string `json:"lineName"`
	ByName   string `json:"byName,omitempty"`
	Seconds  int    `json:"seconds"`
}

func (w *Web) liveDTOs() []liveDTO {
	out := []liveDTO{}
	for _, lc := range w.pbx.Live() {
		name, _ := w.pbx.lineInfo(lc.LineID)
		out = append(out, liveDTO{LiveCall: lc, LineName: name, ByName: w.pbx.extName(lc.By), Seconds: int(time.Since(lc.Start).Seconds())})
	}
	return out
}

// ---------------------------------------------------------------------------
// Dashboard
// ---------------------------------------------------------------------------

func (w *Web) handleDashboard(rw http.ResponseWriter, r *http.Request) {
	lines := w.lineDTOs(w.store.Lines())
	var reg, failed, disabled int
	for _, l := range lines {
		switch l.Status.State {
		case "registered":
			reg++
		case "failed":
			failed++
		case "disabled":
			disabled++
		}
	}
	_, _, today := w.calls.Query(CallQuery{From: w.dayStart(time.Now())})
	exts := []map[string]any{}
	for _, e := range w.store.Extensions() {
		exts = append(exts, map[string]any{"num": e.Num, "name": e.Name, "online": w.pbx.ExtStatusOf(e.Num).Online})
	}
	jsonOK(rw, map[string]any{
		"pbx":     w.pbx.State(),
		"version": w.version,
		"lines":   lines,
		"counts":  map[string]int{"total": len(lines), "registered": reg, "failed": failed, "disabled": disabled},
		"live":    w.liveDTOs(),
		"recent":  w.calls.Recent(6),
		"today":   today,
		"storage": w.pbx.Storage(),
		"exts":    exts,
	})
}

// ---------------------------------------------------------------------------
// Lines
// ---------------------------------------------------------------------------

func (w *Web) formLists() map[string]any {
	anns := []map[string]any{}
	for _, a := range w.store.Announcements() {
		anns = append(anns, map[string]any{"id": a.ID, "name": a.Name, "durMs": a.DurMs})
	}
	exts := []map[string]any{}
	for _, e := range w.store.Extensions() {
		exts = append(exts, map[string]any{"num": e.Num, "name": e.Name})
	}
	return map[string]any{"anns": anns, "exts": exts}
}

func (w *Web) handleLines(rw http.ResponseWriter, r *http.Request) {
	out := w.formLists()
	out["lines"] = w.lineDTOs(w.store.Lines())
	out["recordingsRoot"] = w.pbx.RecordingsRoot()
	jsonOK(rw, out)
}

func (w *Web) handleLineGet(rw http.ResponseWriter, r *http.Request) {
	out := w.formLists()
	out["defaults"] = newLineDefaults()
	out["recordingsRoot"] = w.pbx.RecordingsRoot()
	out["settings"] = map[string]any{"format": w.store.Settings().Format, "retentionDays": w.store.Settings().RetentionDays, "layout": w.store.Settings().FolderLayout}
	if id := r.PathValue("id"); id != "new" {
		l, err := w.store.Line(id)
		if err != nil {
			w.fail(rw, err)
			return
		}
		out["line"] = w.lineDTOs([]Line{l})[0]
	}
	jsonOK(rw, out)
}

func (w *Web) handleLineSave(rw http.ResponseWriter, r *http.Request) {
	in := newLineDefaults()
	if err := decode(r, &in); err != nil {
		w.fail(rw, err)
		return
	}
	in.ID = r.PathValue("id")
	l, err := w.store.SaveLine(in)
	if err != nil {
		w.fail(rw, err)
		return
	}
	jsonOK(rw, w.lineDTOs([]Line{l})[0])
}

func (w *Web) handleLineDelete(rw http.ResponseWriter, r *http.Request) {
	if err := w.store.DeleteLine(r.PathValue("id")); err != nil {
		w.fail(rw, err)
		return
	}
	jsonOK(rw, map[string]bool{"ok": true})
}

func (w *Web) handleLineMode(rw http.ResponseWriter, r *http.Request) {
	var in struct {
		Mode string `json:"mode"`
	}
	if err := decode(r, &in); err != nil {
		w.fail(rw, err)
		return
	}
	l, err := w.store.SetLineMode(r.PathValue("id"), in.Mode)
	if err != nil {
		w.fail(rw, err)
		return
	}
	jsonOK(rw, w.lineDTOs([]Line{l})[0])
}

func (w *Web) handleLineEnabled(rw http.ResponseWriter, r *http.Request) {
	var in struct {
		Enabled bool `json:"enabled"`
	}
	if err := decode(r, &in); err != nil {
		w.fail(rw, err)
		return
	}
	l, err := w.store.SetLineEnabled(r.PathValue("id"), in.Enabled)
	if err != nil {
		w.fail(rw, err)
		return
	}
	jsonOK(rw, w.lineDTOs([]Line{l})[0])
}

func (w *Web) handleLineTest(rw http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	l, err := w.store.Line(id)
	if err != nil {
		w.fail(rw, err)
		return
	}
	if !l.Enabled {
		jsonErr(rw, http.StatusBadRequest, "Hat kapalı; önce açın")
		return
	}
	jsonOK(rw, w.pbx.TestLine(id))
}

type importRow struct {
	Row      int    `json:"row"`
	Name     string `json:"name"`
	Number   string `json:"number"`
	Server   string `json:"server"`
	Username string `json:"username"`
	OK       bool   `json:"ok"`
	Error    string `json:"error,omitempty"`
	line     Line
}

// parseImport reads "ad;numara;sunucu;kullanıcı;şifre[;kimlik kullanıcısı]" rows.
func parseImport(text string, base Line) []importRow {
	var rows []importRow
	for i, raw := range strings.Split(strings.ReplaceAll(text, "\r", ""), "\n") {
		s := strings.TrimSpace(raw)
		if s == "" || strings.HasPrefix(s, "#") {
			continue
		}
		low := strings.ToLower(s)
		if len(rows) == 0 && (strings.Contains(low, "sunucu") || strings.Contains(low, "server")) && strings.Contains(low, "ad") {
			continue // header row
		}
		sep := ","
		switch {
		case strings.Contains(s, ";"):
			sep = ";"
		case strings.Contains(s, "\t"):
			sep = "\t"
		}
		f := strings.Split(s, sep)
		for k := range f {
			f[k] = strings.TrimSpace(strings.Trim(strings.TrimSpace(f[k]), `"`))
		}
		row := importRow{Row: i + 1}
		get := func(k int) string {
			if k < len(f) {
				return f[k]
			}
			return ""
		}
		row.Name, row.Number, row.Server, row.Username = get(0), get(1), get(2), get(3)
		l := clone(base)
		l.Name, l.Number, l.Server, l.Username, l.Password, l.AuthUser = get(0), get(1), get(2), get(3), get(4), get(5)
		if len(f) < 5 {
			row.Error = "En az 5 sütun gerekli: ad;numara;sunucu;kullanıcı;şifre"
		} else if l.Password == "" {
			row.Error = "Şifre eksik"
		}
		row.line = l
		rows = append(rows, row)
	}
	return rows
}

func (w *Web) handleLineImport(rw http.ResponseWriter, r *http.Request) {
	var in struct {
		Text   string   `json:"text"`
		Mode   string   `json:"mode"`
		AnnID  string   `json:"annId"`
		Exts   []string `json:"exts"`
		DryRun bool     `json:"dryRun"`
	}
	if err := decode(r, &in); err != nil {
		w.fail(rw, err)
		return
	}
	base := newLineDefaults()
	if in.Mode != "" {
		base.Mode = in.Mode
	}
	base.AnnID = in.AnnID
	base.Exts = in.Exts
	rows := parseImport(in.Text, base)
	if len(rows) == 0 {
		jsonErr(rw, http.StatusBadRequest, "Yapıştırılan metinde hat bulunamadı")
		return
	}
	names := map[string]int{}
	var good []Line
	for i := range rows {
		if rows[i].Error != "" {
			continue
		}
		n, err := w.store.ValidateLine(rows[i].line)
		if err != nil {
			rows[i].Error = err.Error()
			continue
		}
		key := strings.ToLower(n.Name)
		if prev, dup := names[key]; dup {
			rows[i].Error = fmt.Sprintf("Ad %d. satırla aynı", prev)
			continue
		}
		names[key] = rows[i].Row
		rows[i].OK = true
		good = append(good, rows[i].line)
	}
	added := 0
	if !in.DryRun && len(good) > 0 {
		saved, err := w.store.ImportLines(good)
		if err != nil {
			w.fail(rw, err)
			return
		}
		added = len(saved)
	}
	jsonOK(rw, map[string]any{"rows": rows, "added": added, "valid": len(good)})
}

// handleLinesBulk applies one action to many lines: update (only the given
// fields), enable, disable or delete.
func (w *Web) handleLinesBulk(rw http.ResponseWriter, r *http.Request) {
	var in struct {
		IDs     []string `json:"ids"`
		Action  string   `json:"action"`
		Changes struct {
			Mode       *string   `json:"mode"`
			AnnID      *string   `json:"annId"`
			Exts       *[]string `json:"exts"`
			RingSec    *int      `json:"ringSec"`
			NoAnswer   *string   `json:"noAnswer"`
			NoAnsAnnID *string   `json:"noAnsAnnId"`
			After      *string   `json:"after"`
			VMMaxSec   *int      `json:"vmMaxSec"`
			Record     *bool     `json:"record"`
			CIDPrefix  *bool     `json:"cidPrefix"`
		} `json:"changes"`
	}
	if err := decode(r, &in); err != nil {
		w.fail(rw, err)
		return
	}
	if len(in.IDs) == 0 {
		jsonErr(rw, http.StatusBadRequest, "Önce hat seçin")
		return
	}
	var n int
	var err error
	switch in.Action {
	case "delete":
		n, err = w.store.DeleteLines(in.IDs)
	case "enable", "disable":
		on := in.Action == "enable"
		n, err = w.store.BulkUpdate(in.IDs, func(l *Line) { l.Enabled = on })
	case "update":
		c := in.Changes
		n, err = w.store.BulkUpdate(in.IDs, func(l *Line) {
			if c.Mode != nil {
				l.Mode = *c.Mode
			}
			if c.AnnID != nil {
				l.AnnID = *c.AnnID
			}
			if c.Exts != nil {
				l.Exts = append([]string{}, (*c.Exts)...)
			}
			if c.RingSec != nil {
				l.RingSec = *c.RingSec
			}
			if c.NoAnswer != nil {
				l.NoAnswer = *c.NoAnswer
			}
			if c.NoAnsAnnID != nil {
				l.NoAnsAnnID = *c.NoAnsAnnID
			}
			if c.After != nil {
				l.After = *c.After
			}
			if c.VMMaxSec != nil {
				l.VMMaxSec = *c.VMMaxSec
			}
			if c.Record != nil {
				l.Record = *c.Record
			}
			if c.CIDPrefix != nil {
				l.CIDPrefix = *c.CIDPrefix
			}
		})
	default:
		jsonErr(rw, http.StatusBadRequest, "Bilinmeyen toplu işlem")
		return
	}
	if err != nil {
		w.fail(rw, err)
		return
	}
	w.ev.Add("info", "Hatlar", fmt.Sprintf("Toplu işlem (%s): %d hat", in.Action, n))
	jsonOK(rw, map[string]int{"count": n})
}

// ---------------------------------------------------------------------------
// Extensions
// ---------------------------------------------------------------------------

func (w *Web) handleExts(rw http.ResponseWriter, r *http.Request) {
	lines := w.store.Lines()
	answered := w.calls.CountByExt(w.dayStart(time.Now()))
	st := w.store.Settings()
	list := []map[string]any{}
	for _, e := range w.store.Extensions() {
		var ln []string
		for _, l := range lines {
			if (l.Mode == "ring" || l.Mode == "annring") && contains(l.Exts, e.Num) {
				ln = append(ln, l.Name)
			}
		}
		es := w.pbx.ExtStatusOf(e.Num)
		list = append(list, map[string]any{
			"num": e.Num, "name": e.Name, "status": es, "lastSeenText": sinceText(es.LastSeen),
			"todayAnswered": answered[e.Num], "lines": ln, "created": e.Created,
		})
	}
	transports := []string{}
	if st.UDP {
		transports = append(transports, "UDP")
	}
	if st.TCP {
		transports = append(transports, "TCP")
	}
	jsonOK(rw, map[string]any{
		"exts":   list,
		"server": map[string]any{"host": lanIP(), "port": st.SIPPort, "transports": transports, "lanOnly": st.ExtLANOnly},
	})
}

func (w *Web) handleExtSave(rw http.ResponseWriter, r *http.Request) {
	var in Extension
	if err := decode(r, &in); err != nil {
		w.fail(rw, err)
		return
	}
	e, err := w.store.SaveExtension(r.PathValue("num"), in)
	if err != nil {
		w.fail(rw, err)
		return
	}
	jsonOK(rw, e)
}

func (w *Web) handleExtDelete(rw http.ResponseWriter, r *http.Request) {
	if err := w.store.DeleteExtension(r.PathValue("num")); err != nil {
		w.fail(rw, err)
		return
	}
	jsonOK(rw, map[string]bool{"ok": true})
}

func (w *Web) handleExtSecret(rw http.ResponseWriter, r *http.Request) {
	e, err := w.store.Extension(r.PathValue("num"))
	if err != nil {
		w.fail(rw, err)
		return
	}
	jsonOK(rw, map[string]string{"password": e.Password})
}

func (w *Web) handleExtResetPassword(rw http.ResponseWriter, r *http.Request) {
	e, err := w.store.ResetExtPassword(r.PathValue("num"))
	if err != nil {
		w.fail(rw, err)
		return
	}
	w.ev.Add("info", "Dahili "+e.Num, "Şifre yenilendi")
	jsonOK(rw, map[string]string{"password": e.Password})
}

// ---------------------------------------------------------------------------
// Announcements and text-to-speech
// ---------------------------------------------------------------------------

func (w *Web) annPath(id string) string { return filepath.Join(w.dataDir, "sounds", id+".wav") }

func (w *Web) handleAnns(rw http.ResponseWriter, r *http.Request) {
	list := []map[string]any{}
	for _, a := range w.store.Announcements() {
		users := w.store.AnnouncementUsers(a.ID)
		if users == nil {
			users = []string{}
		}
		list = append(list, map[string]any{"ann": a, "users": users})
	}
	jsonOK(rw, map[string]any{"anns": list, "voices": w.tts.Voices(), "ttsAvailable": w.tts.Available()})
}

func (w *Web) saveAnnouncement(a Announcement, wav []byte, pcm PCM) (Announcement, error) {
	a.ID = newID("a")
	a.DurMs = int(pcm.Duration().Milliseconds())
	if err := writeFileAtomic(w.annPath(a.ID), wav, 0o644); err != nil {
		return a, err
	}
	saved, err := w.store.AddAnnouncement(a)
	if err != nil {
		_ = os.Remove(w.annPath(a.ID))
		return a, err
	}
	return saved, nil
}

func (w *Web) handleAnnCreate(rw http.ResponseWriter, r *http.Request) {
	ct, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if ct == "multipart/form-data" {
		r.Body = http.MaxBytesReader(rw, r.Body, 64<<20)
		if err := r.ParseMultipartForm(16 << 20); err != nil {
			jsonErr(rw, http.StatusBadRequest, "Dosya en fazla 60 MB olabilir")
			return
		}
		f, hdr, err := r.FormFile("audio")
		if err != nil {
			jsonErr(rw, http.StatusBadRequest, "Ses dosyası gelmedi")
			return
		}
		defer f.Close()
		pcm, err := ReadWAV(f)
		if err != nil {
			jsonErr(rw, http.StatusBadRequest, "Ses okunamadı: "+err.Error())
			return
		}
		out, err := PrepareAnnouncement(pcm)
		if err != nil {
			w.fail(rw, err)
			return
		}
		src := r.FormValue("source")
		if src != "mic" {
			src = "file"
		}
		fileName := r.FormValue("fileName")
		if fileName == "" && hdr != nil && src == "file" {
			fileName = hdr.Filename
		}
		a, err := w.saveAnnouncement(Announcement{Name: r.FormValue("name"), Source: src, FileName: filepath.Base(fileName)}, EncodeWAV16(out), out)
		if err != nil {
			w.fail(rw, err)
			return
		}
		jsonOK(rw, a)
		return
	}
	var in struct {
		Name  string  `json:"name"`
		Text  string  `json:"text"`
		Voice string  `json:"voice"`
		Speed float64 `json:"speed"`
	}
	if err := decode(r, &in); err != nil {
		w.fail(rw, err)
		return
	}
	if _, err := cleanName(in.Name, 60, "Anons adı"); err != nil {
		w.fail(rw, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 4*time.Minute)
	defer cancel()
	wav, err := w.tts.Synthesize(ctx, in.Text, in.Voice, in.Speed)
	if err != nil {
		w.fail(rw, err)
		return
	}
	pcm, _ := ReadWAV(bytes.NewReader(wav))
	a, err := w.saveAnnouncement(Announcement{Name: in.Name, Source: "tts", Text: strings.TrimSpace(in.Text), Voice: in.Voice, Speed: in.Speed}, wav, pcm)
	if err != nil {
		w.fail(rw, err)
		return
	}
	jsonOK(rw, a)
}

func (w *Web) handleAnnRename(rw http.ResponseWriter, r *http.Request) {
	var in struct {
		Name string `json:"name"`
	}
	if err := decode(r, &in); err != nil {
		w.fail(rw, err)
		return
	}
	a, err := w.store.RenameAnnouncement(r.PathValue("id"), in.Name)
	if err != nil {
		w.fail(rw, err)
		return
	}
	jsonOK(rw, a)
}

func (w *Web) handleAnnDelete(rw http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := w.store.DeleteAnnouncement(id); err != nil {
		w.fail(rw, err)
		return
	}
	_ = os.Remove(w.annPath(id))
	jsonOK(rw, map[string]bool{"ok": true})
}

func (w *Web) handleAnnAudio(rw http.ResponseWriter, r *http.Request) {
	a, err := w.store.Announcement(r.PathValue("id"))
	if err != nil {
		w.fail(rw, err)
		return
	}
	f, err := os.Open(w.annPath(a.ID))
	if err != nil {
		w.fail(rw, errNotFound)
		return
	}
	defer f.Close()
	st, _ := f.Stat()
	rw.Header().Set("Content-Type", "audio/wav")
	if r.URL.Query().Get("download") == "1" {
		rw.Header().Set("Content-Disposition", contentDisposition(slug(a.Name)+".wav"))
	}
	http.ServeContent(rw, r, a.ID+".wav", st.ModTime(), f)
}

func (w *Web) handleTTSPreview(rw http.ResponseWriter, r *http.Request) {
	var in struct {
		Text  string  `json:"text"`
		Voice string  `json:"voice"`
		Speed float64 `json:"speed"`
	}
	if err := decode(r, &in); err != nil {
		w.fail(rw, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 4*time.Minute)
	defer cancel()
	wav, err := w.tts.Synthesize(ctx, in.Text, in.Voice, in.Speed)
	if err != nil {
		w.fail(rw, err)
		return
	}
	rw.Header().Set("Content-Type", "audio/wav")
	rw.Header().Set("Content-Length", strconv.Itoa(len(wav)))
	rw.Write(wav)
}

func (w *Web) handleVoiceDownload(rw http.ResponseWriter, r *http.Request) {
	if err := w.tts.Download(r.PathValue("id")); err != nil {
		w.fail(rw, err)
		return
	}
	jsonOK(rw, map[string]bool{"ok": true})
}

func (w *Web) handleVoiceDelete(rw http.ResponseWriter, r *http.Request) {
	if err := w.tts.DeleteVoice(r.PathValue("id")); err != nil {
		w.fail(rw, err)
		return
	}
	jsonOK(rw, map[string]bool{"ok": true})
}

// ---------------------------------------------------------------------------
// Calls and recordings
// ---------------------------------------------------------------------------

func (w *Web) parseCallQuery(r *http.Request) CallQuery {
	q := r.URL.Query()
	now := time.Now()
	today := w.dayStart(now)
	cq := CallQuery{LineID: q.Get("line"), Result: q.Get("result"), Search: strings.TrimSpace(q.Get("q"))}
	switch q.Get("range") {
	case "yesterday":
		cq.From, cq.To = today.AddDate(0, 0, -1), today
	case "7d":
		cq.From = today.AddDate(0, 0, -6)
	case "30d":
		cq.From = today.AddDate(0, 0, -29)
	case "all":
	case "custom":
		if t, err := time.ParseInLocation("2006-01-02", q.Get("from"), w.loc()); err == nil {
			cq.From = t
		}
		if t, err := time.ParseInLocation("2006-01-02", q.Get("to"), w.loc()); err == nil {
			cq.To = t.AddDate(0, 0, 1)
		}
	default:
		cq.From = today
	}
	cq.Limit, _ = strconv.Atoi(q.Get("limit"))
	if cq.Limit <= 0 || cq.Limit > 500 {
		cq.Limit = 50
	}
	cq.Offset, _ = strconv.Atoi(q.Get("offset"))
	if cq.Offset < 0 {
		cq.Offset = 0
	}
	return cq
}

func (w *Web) handleCalls(rw http.ResponseWriter, r *http.Request) {
	cq := w.parseCallQuery(r)
	items, total, sum := w.calls.Query(cq)
	lines := []map[string]string{}
	for _, l := range w.store.Lines() {
		lines = append(lines, map[string]string{"id": l.ID, "name": l.Name})
	}
	jsonOK(rw, map[string]any{
		"items": items, "total": total, "summary": sum, "live": w.liveDTOs(), "lines": lines,
		"retentionDays": w.store.Settings().RetentionDays, "recordingsRoot": w.pbx.RecordingsRoot(),
	})
}

func contentDisposition(name string) string {
	ascii := strings.Map(func(r rune) rune {
		if r < 32 || r > 126 || r == '"' || r == '\\' {
			return '_'
		}
		return r
	}, name)
	return fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, ascii, strings.ReplaceAll(urlPathEscape(name), "%2F", "_"))
}

func urlPathEscape(s string) string {
	var sb strings.Builder
	for _, b := range []byte(s) {
		if (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '-' || b == '_' || b == '.' {
			sb.WriteByte(b)
		} else {
			fmt.Fprintf(&sb, "%%%02X", b)
		}
	}
	return sb.String()
}

func (w *Web) handleCallAudio(rw http.ResponseWriter, r *http.Request) {
	rec, ok := w.calls.Get(r.PathValue("id"))
	if !ok {
		w.fail(rw, errNotFound)
		return
	}
	path, err := w.pbx.RecordingPath(rec)
	if err != nil {
		w.fail(rw, err)
		return
	}
	f, err := os.Open(path)
	if err != nil {
		jsonErr(rw, http.StatusNotFound, "Kayıt dosyası bulunamadı (silinmiş ya da taşınmış olabilir)")
		return
	}
	defer f.Close()
	st, _ := f.Stat()
	if strings.HasSuffix(path, ".mp3") {
		rw.Header().Set("Content-Type", "audio/mpeg")
	} else {
		rw.Header().Set("Content-Type", "audio/wav")
	}
	if r.URL.Query().Get("download") == "1" {
		rw.Header().Set("Content-Disposition", contentDisposition(downloadName(rec, path, w.loc())))
	}
	http.ServeContent(rw, r, filepath.Base(path), st.ModTime(), f)
}

func downloadName(rec CallRecord, path string, loc *time.Location) string {
	return rec.Start.In(loc).Format("2006-01-02_150405") + "_" + slug(rec.LineName) + "_" + callerForFile(rec.Caller) + filepath.Ext(path)
}

func (w *Web) handleCallsZip(rw http.ResponseWriter, r *http.Request) {
	var in struct {
		IDs []string `json:"ids"`
	}
	if err := decode(r, &in); err != nil {
		w.fail(rw, err)
		return
	}
	if len(in.IDs) == 0 || len(in.IDs) > 2000 {
		jsonErr(rw, http.StatusBadRequest, "1–2000 arama seçin")
		return
	}
	loc := w.loc()
	type item struct{ path, name string }
	var items []item
	for _, id := range in.IDs {
		rec, ok := w.calls.Get(id)
		if !ok {
			continue
		}
		p, err := w.pbx.RecordingPath(rec)
		if err != nil {
			continue
		}
		if _, err := os.Stat(p); err == nil {
			items = append(items, item{p, downloadName(rec, p, loc)})
		}
	}
	if len(items) == 0 {
		jsonErr(rw, http.StatusBadRequest, "Seçilen aramaların kaydı yok")
		return
	}
	rw.Header().Set("Content-Type", "application/zip")
	rw.Header().Set("Content-Disposition", contentDisposition("sip-kayitlari-"+time.Now().In(loc).Format("20060102-1504")+".zip"))
	zw := zip.NewWriter(rw)
	for _, it := range items {
		f, err := os.Open(it.path)
		if err != nil {
			continue
		}
		st, _ := f.Stat()
		h := &zip.FileHeader{Name: it.name, Method: zip.Store, Modified: st.ModTime()}
		zf, err := zw.CreateHeader(h)
		if err == nil {
			_, _ = io.Copy(zf, f)
		}
		f.Close()
	}
	_ = zw.Close()
}

var resultNames = map[string]string{"answered": "Cevaplandı", "missed": "Cevapsız", "voicemail": "Sesli mesaj", "announcement": "Sadece anons", "busy": "Meşgul verildi", "interrupted": "Yarıda kaldı"}

func (w *Web) handleCallsCSV(rw http.ResponseWriter, r *http.Request) {
	cq := w.parseCallQuery(r)
	cq.Limit, cq.Offset = 0, 0
	items, _, _ := w.calls.Query(cq)
	loc := w.loc()
	rw.Header().Set("Content-Type", "text/csv; charset=utf-8")
	rw.Header().Set("Content-Disposition", contentDisposition("aramalar-"+time.Now().In(loc).Format("20060102")+".csv"))
	rw.Write([]byte("\xEF\xBB\xBF"))
	cw := csv.NewWriter(rw)
	cw.Comma = ';'
	_ = cw.Write([]string{"Tarih", "Saat", "Hat", "Hat numarası", "Arayan", "Arayan adı", "Sonuç", "Açan dahili", "Süre (sn)", "Konuşma (sn)", "Kayıt dosyası"})
	for _, c := range items {
		t := c.Start.In(loc)
		by := c.AnsweredBy
		if c.AnsweredByName != "" {
			by += " " + c.AnsweredByName
		}
		_ = cw.Write([]string{t.Format("02.01.2006"), t.Format("15:04:05"), c.LineName, c.LineNumber, c.Caller, c.CallerName,
			resultNames[c.Result], by, strconv.Itoa(c.DurSec), strconv.Itoa(c.TalkSec), c.Rec})
	}
	cw.Flush()
}

// ---------------------------------------------------------------------------
// Settings and system
// ---------------------------------------------------------------------------

func listShares() []string {
	var out []string
	for _, root := range volumeRoots() {
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, e := range entries {
			n := e.Name()
			if !e.IsDir() || strings.HasPrefix(n, ".") || strings.HasPrefix(n, "@") || n == "lost+found" {
				continue
			}
			out = append(out, filepath.Join(root, n))
		}
	}
	return out
}

func timezoneName(s Settings) string {
	if s.Timezone != "" {
		return s.Timezone
	}
	if tz := os.Getenv("TZ"); tz != "" {
		return tz + " (sistem)"
	}
	if target, err := os.Readlink("/etc/localtime"); err == nil {
		if i := strings.Index(target, "zoneinfo/"); i >= 0 {
			return target[i+len("zoneinfo/"):] + " (sistem)"
		}
	}
	name, off := time.Now().Zone()
	return fmt.Sprintf("Sistem (%s, UTC%+d)", name, off/3600)
}

func (w *Web) handleSettingsGet(rw http.ResponseWriter, r *http.Request) {
	st := w.store.Settings()
	blocked := []map[string]any{}
	for ip, until := range w.pbx.BlockedIPs() {
		blocked = append(blocked, map[string]any{"ip": ip, "until": until})
	}
	sort.Slice(blocked, func(i, j int) bool { return blocked[i]["ip"].(string) < blocked[j]["ip"].(string) })
	zones := []string{}
	for z := range toneZones {
		zones = append(zones, z)
	}
	sort.Strings(zones)
	jsonOK(rw, map[string]any{
		"settings":       st,
		"defaultShare":   defaultShareFolder(),
		"recordingsRoot": w.pbx.RecordingsRoot(),
		"shares":         listShares(),
		"timezoneName":   timezoneName(st),
		"detectedNets":   detectLocalNets(),
		"lanIPs":         lanAddresses(),
		"pbx":            w.pbx.State(),
		"blocked":        blocked,
		"storage":        w.pbx.Storage(),
		"toneZones":      zones,
		"version":        w.version,
	})
}

func (w *Web) handleSettingsPut(rw http.ResponseWriter, r *http.Request) {
	cur := w.store.Settings()
	in := cur
	if err := decode(r, &in); err != nil {
		w.fail(rw, err)
		return
	}
	in.SIPDebug = cur.SIPDebug
	if in.ShareFolder != "" {
		abs, err := filepath.Abs(in.ShareFolder)
		if err != nil {
			w.fail(rw, verr("Klasör yolu geçersiz"))
			return
		}
		in.ShareFolder = abs
		if err := os.MkdirAll(abs, 0o775); err != nil {
			w.fail(rw, verr("Kayıt klasörü oluşturulamadı: %v", err))
			return
		}
	}
	st, err := w.store.SaveSettings(in)
	if err != nil {
		w.fail(rw, err)
		return
	}
	restart := st.SIPPort != cur.SIPPort || st.UDP != cur.UDP || st.TCP != cur.TCP || st.RTPStart != cur.RTPStart || st.RTPEnd != cur.RTPEnd || st.ToneZone != cur.ToneZone
	jsonOK(rw, map[string]any{"settings": st, "restart": restart})
}

func (w *Web) handleRestart(rw http.ResponseWriter, r *http.Request) {
	w.ev.Add("warn", "Santral", "Asterisk elle yeniden başlatıldı")
	w.pbx.Restart()
	jsonOK(rw, map[string]bool{"ok": true})
}

func (w *Web) handleSIPDebug(rw http.ResponseWriter, r *http.Request) {
	var in struct {
		On bool `json:"on"`
	}
	if err := decode(r, &in); err != nil {
		w.fail(rw, err)
		return
	}
	if err := w.pbx.SetSIPDebug(in.On); err != nil {
		w.fail(rw, err)
		return
	}
	jsonOK(rw, map[string]bool{"on": in.On})
}

func (w *Web) handleUnblock(rw http.ResponseWriter, r *http.Request) {
	var in struct {
		IP string `json:"ip"`
	}
	if err := decode(r, &in); err != nil {
		w.fail(rw, err)
		return
	}
	w.pbx.Unblock(in.IP)
	w.ev.Add("info", "Güvenlik", in.IP+" engeli kaldırıldı")
	jsonOK(rw, map[string]bool{"ok": true})
}

func (w *Web) handleLog(rw http.ResponseWriter, r *http.Request) {
	out := map[string]any{"events": w.ev.Recent(300), "sipDebug": w.store.Settings().SIPDebug}
	if r.URL.Query().Get("raw") == "1" {
		out["raw"] = w.pbx.AsteriskLog(96 << 10)
	}
	jsonOK(rw, out)
}

type backupFile struct {
	App     string            `json:"app"`
	Version string            `json:"version"`
	Created time.Time         `json:"created"`
	Config  Config            `json:"config"`
	Sounds  map[string]string `json:"sounds"`
}

func (w *Web) handleBackupGet(rw http.ResponseWriter, r *http.Request) {
	cfg := w.store.Snapshot()
	cfg.PasswordHash, cfg.AMISecret = "", ""
	b := backupFile{App: "sipsantral", Version: w.version, Created: time.Now(), Config: cfg, Sounds: map[string]string{}}
	for _, a := range cfg.Announcements {
		if data, err := os.ReadFile(w.annPath(a.ID)); err == nil {
			b.Sounds[a.ID] = base64.StdEncoding.EncodeToString(data)
		}
	}
	rw.Header().Set("Content-Type", "application/json")
	rw.Header().Set("Content-Disposition", contentDisposition("sip-santral-yedek-"+time.Now().In(w.loc()).Format("20060102")+".json"))
	enc := json.NewEncoder(rw)
	enc.SetIndent("", " ")
	_ = enc.Encode(b)
}

func (w *Web) handleBackupRestore(rw http.ResponseWriter, r *http.Request) {
	var b backupFile
	dec := json.NewDecoder(http.MaxBytesReader(rw, r.Body, 128<<20))
	if err := dec.Decode(&b); err != nil || b.App != "sipsantral" {
		jsonErr(rw, http.StatusBadRequest, "Bu dosya bir SIP Santral yedeği değil")
		return
	}
	for _, a := range b.Config.Announcements {
		data, err := base64.StdEncoding.DecodeString(b.Sounds[a.ID])
		if err != nil || len(data) < 44 {
			jsonErr(rw, http.StatusBadRequest, "Yedekte “"+a.Name+"” anonsunun sesi eksik")
			return
		}
		if _, err := ReadWAV(bytes.NewReader(data)); err != nil {
			jsonErr(rw, http.StatusBadRequest, "Yedekteki ses bozuk: "+a.Name)
			return
		}
		if strings.ContainsAny(a.ID, "/\\.") || a.ID == "" {
			jsonErr(rw, http.StatusBadRequest, "Yedekte geçersiz anons kimliği")
			return
		}
		if err := writeFileAtomic(w.annPath(a.ID), data, 0o644); err != nil {
			w.fail(rw, err)
			return
		}
	}
	if err := w.store.Restore(b.Config); err != nil {
		w.fail(rw, err)
		return
	}
	w.ev.Add("warn", "Yedek", "Ayarlar yedekten geri yüklendi")
	jsonOK(rw, map[string]bool{"ok": true})
}
