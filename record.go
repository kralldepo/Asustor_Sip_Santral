package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

type recJob struct {
	rec    CallRecord
	base   string // MixMonitor file name without .wav ("<epoch>-<line>-<uniqueid>")
	queued time.Time
}

// volumeRoots lists /volume1, /volume2 ... (not the ADM system volume0).
var volumeRoots = func() []string {
	matches, _ := filepath.Glob("/volume[0-9]*")
	var out []string
	for _, m := range matches {
		if m == "/volume0" {
			continue
		}
		if st, err := os.Stat(m); err == nil && st.IsDir() {
			out = append(out, m)
		}
	}
	sort.Strings(out)
	return out
}

// RecordingsRoot is the folder recordings are written to.
func (p *PBX) RecordingsRoot() string {
	if s := p.store.Settings().ShareFolder; s != "" {
		return s
	}
	return defaultShareFolder()
}

func defaultShareFolder() string {
	roots := volumeRoots()
	for _, r := range roots {
		d := filepath.Join(r, "SipKayitlari")
		if st, err := os.Stat(d); err == nil && st.IsDir() {
			return d
		}
	}
	if len(roots) > 0 {
		return filepath.Join(roots[0], "SipKayitlari")
	}
	return filepath.Join(".", "SipKayitlari")
}

func (p *PBX) monitorDir() (string, error) { return p.rf.Path("/var/spool/asterisk/monitor") }

func (p *PBX) recorderLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case job := <-p.recJobs:
			rec := p.processRecording(ctx, job)
			if err := p.calls.Add(rec); err != nil {
				p.log.Printf("arama kaydı yazılamadı: %v", err)
			}
		}
	}
}

// processRecording waits for MixMonitor to finish, encodes and files the
// recording. It always returns the (possibly updated) call record.
func (p *PBX) processRecording(ctx context.Context, job recJob) CallRecord {
	rec := job.rec
	dir, err := p.monitorDir()
	if err != nil {
		return rec
	}
	wav := filepath.Join(dir, job.base+".wav")
	done := filepath.Join(dir, job.base+".done")
	deadline := time.Now().Add(90 * time.Second)
	lastSize, stableSince := int64(-1), time.Now()
	for time.Now().Before(deadline) {
		if _, err := os.Stat(done); err == nil {
			break
		}
		st, err := os.Stat(wav)
		if err == nil {
			if st.Size() != lastSize {
				lastSize, stableSince = st.Size(), time.Now()
			} else if time.Since(stableSince) > 8*time.Second {
				break
			}
		} else if time.Since(job.queued) > 20*time.Second {
			break
		}
		select {
		case <-ctx.Done():
			return rec
		case <-time.After(500 * time.Millisecond):
		}
	}
	defer os.Remove(done)
	if _, err := os.Stat(wav); err != nil {
		return rec
	}
	dur := wavFileDuration(wav)
	keep := rec.Result == "answered" || rec.Result == "voicemail" || rec.Result == "announcement" || rec.Result == "interrupted"
	if !keep || dur < 700*time.Millisecond {
		_ = os.Remove(wav)
		return rec
	}
	if err := p.fileRecording(&rec, wav, dur); err != nil {
		p.log.Printf("kayıt dosyalanamadı (%s): %v", job.base, err)
		p.ev.Add("error", rec.LineName, "Kayıt kaydedilemedi: "+err.Error())
	}
	return rec
}

func callerForFile(c string) string {
	var sb strings.Builder
	for i, r := range c {
		if (r >= '0' && r <= '9') || (r == '+' && i == 0) {
			sb.WriteRune(r)
		}
	}
	if sb.Len() == 0 {
		return "gizli"
	}
	return sb.String()
}

// recordingRelPath builds the path below the recordings folder.
func recordingRelPath(layout string, t time.Time, lineName, caller, ext string) string {
	line := slug(lineName)
	base := t.Format("150405") + "_" + callerForFile(caller) + "." + ext
	switch layout {
	case "date-line":
		return filepath.Join(t.Format("2006"), t.Format("01"), t.Format("02"), line, base)
	case "flat":
		return t.Format("2006-01-02") + "_" + t.Format("150405") + "_" + line + "_" + callerForFile(caller) + "." + ext
	default:
		return filepath.Join(line, t.Format("2006"), t.Format("01"), t.Format("02"), base)
	}
}

func (p *PBX) fileRecording(rec *CallRecord, wav string, dur time.Duration) error {
	st := p.store.Settings()
	root := p.RecordingsRoot()
	if err := os.MkdirAll(root, 0o775); err != nil {
		return err
	}
	owner := ownerOf(root)
	src, ext := wav, "wav"
	if st.Format == "mp3" {
		mp3 := strings.TrimSuffix(wav, ".wav") + ".mp3"
		if err := p.encodeMP3(wav, mp3); err != nil {
			p.log.Printf("MP3'e çevrilemedi, WAV saklanıyor: %v", err)
			_ = os.Remove(mp3)
		} else {
			src, ext = mp3, "mp3"
		}
	}
	loc := st.Location()
	rel := recordingRelPath(st.FolderLayout, rec.Start.In(loc), rec.LineName, rec.Caller, ext)
	dst := filepath.Join(root, rel)
	for i := 2; ; i++ {
		if _, err := os.Lstat(dst); os.IsNotExist(err) {
			break
		}
		rel = strings.TrimSuffix(rel, "."+ext)
		rel = strings.TrimSuffix(rel, "_"+strconv.Itoa(i-1)) + "_" + strconv.Itoa(i) + "." + ext
		dst = filepath.Join(root, rel)
	}
	if err := mkdirAllOwned(filepath.Dir(dst), root, owner); err != nil {
		return err
	}
	if err := moveFile(src, dst, 0o664); err != nil {
		return err
	}
	_ = os.Chmod(dst, 0o664)
	if owner != nil {
		_ = os.Lchown(dst, owner[0], owner[1])
	}
	if src != wav {
		_ = os.Remove(wav)
	}
	if fi, err := os.Stat(dst); err == nil {
		rec.RecBytes = fi.Size()
		storageCache.Lock()
		if storageCache.info.Root == root {
			storageCache.info.Used += fi.Size()
		}
		storageCache.Unlock()
	}
	rec.Rec = filepath.ToSlash(rel)
	rec.RecRoot = root
	rec.RecSec = int(dur.Round(time.Second).Seconds())
	return nil
}

var lameMu sync.Mutex

func (p *PBX) encodeMP3(wav, mp3 string) error {
	lameMu.Lock() // one encoder at a time keeps the NAS responsive
	defer lameMu.Unlock()
	if !canRunPBX {
		return fmt.Errorf("MP3 kodlayıcı yalnız NAS üzerinde")
	}
	in := "/var/spool/asterisk/monitor/" + filepath.Base(wav)
	out := "/var/spool/asterisk/monitor/" + filepath.Base(mp3)
	cmd := p.rf.chrootCmd("/usr/bin/lame", "--quiet", "--resample", "16", "-m", "m", "-b", "32", in, out)
	done := make(chan error, 1)
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			return err
		}
	case <-time.After(10 * time.Minute):
		killGroup(cmd, syscall.SIGKILL)
		return fmt.Errorf("lame zaman aşımı")
	}
	if fi, err := os.Stat(mp3); err != nil || fi.Size() == 0 {
		return fmt.Errorf("MP3 oluşmadı")
	}
	return nil
}

func ownerOf(path string) *[2]int {
	st, err := os.Stat(path)
	if err != nil {
		return nil
	}
	if sys, ok := st.Sys().(*syscall.Stat_t); ok {
		return &[2]int{int(sys.Uid), int(sys.Gid)}
	}
	return nil
}

// mkdirAllOwned creates dir (below root) giving new folders root's owner.
func mkdirAllOwned(dir, root string, owner *[2]int) error {
	rel, err := filepath.Rel(root, dir)
	if err != nil || strings.HasPrefix(rel, "..") {
		return os.MkdirAll(dir, 0o775)
	}
	cur := root
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		if part == "." || part == "" {
			continue
		}
		cur = filepath.Join(cur, part)
		if _, err := os.Stat(cur); err == nil {
			continue
		}
		if err := os.Mkdir(cur, 0o775); err != nil && !os.IsExist(err) {
			return err
		}
		_ = os.Chmod(cur, 0o775)
		if owner != nil {
			_ = os.Lchown(cur, owner[0], owner[1])
		}
	}
	return nil
}

// processOrphans files recordings left behind by a crash or power loss.
func (p *PBX) processOrphans() {
	dir, err := p.monitorDir()
	if err != nil {
		return
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		name := e.Name()
		if strings.HasSuffix(name, ".done") || strings.HasSuffix(name, ".mp3") {
			if _, err := os.Stat(filepath.Join(dir, strings.TrimSuffix(strings.TrimSuffix(name, ".done"), ".mp3")+".wav")); os.IsNotExist(err) {
				_ = os.Remove(filepath.Join(dir, name))
			}
			continue
		}
		if !strings.HasSuffix(name, ".wav") {
			continue
		}
		base := strings.TrimSuffix(name, ".wav")
		parts := strings.SplitN(base, "-", 3)
		if len(parts) != 3 {
			_ = os.Remove(filepath.Join(dir, name))
			continue
		}
		start := epochTime(parts[0])
		if _, done := p.calls.Get(parts[2]); done {
			_ = os.Remove(filepath.Join(dir, name))
			continue
		}
		rec := p.baseRecord(parts[2], parts[1], "", "", "", start, start.Add(wavFileDuration(filepath.Join(dir, name))))
		rec.Caller = "Bilinmiyor"
		rec.Result = "interrupted"
		_ = os.WriteFile(filepath.Join(dir, base+".done"), nil, 0o644)
		select {
		case p.recJobs <- recJob{rec: rec, base: base, queued: time.Now()}:
		default:
		}
	}
}

// pruneRecordings deletes calls and recordings older than the retention.
func (p *PBX) pruneRecordings() {
	days := p.store.Settings().RetentionDays
	if days <= 0 {
		return
	}
	cutoff := time.Now().AddDate(0, 0, -days)
	dirs := map[string]string{}
	n := p.calls.Prune(cutoff, func(r CallRecord) {
		root := r.RecRoot
		if root == "" {
			root = p.RecordingsRoot()
		}
		path := filepath.Join(root, filepath.FromSlash(r.Rec))
		if err := os.Remove(path); err == nil {
			dirs[filepath.Dir(path)] = root
		}
	})
	for d, root := range dirs { // drop now-empty date folders
		for d != root && strings.HasPrefix(d, root) {
			if err := os.Remove(d); err != nil {
				break
			}
			d = filepath.Dir(d)
		}
	}
	if n > 0 {
		p.ev.Add("info", "Kayıtlar", fmt.Sprintf("%d günden eski %d arama ve kaydı silindi", days, n))
	}
}

// RecordingPath resolves the absolute path of a call's recording.
func (p *PBX) RecordingPath(r CallRecord) (string, error) {
	if r.Rec == "" {
		return "", errNotFound
	}
	root := r.RecRoot
	if root == "" {
		root = p.RecordingsRoot()
	}
	full := filepath.Join(root, filepath.FromSlash(r.Rec))
	if rel, err := filepath.Rel(root, full); err != nil || strings.HasPrefix(rel, "..") {
		return "", errNotFound
	}
	return full, nil
}

// StorageInfo reports the size of the recordings folder and free space.
type StorageInfo struct {
	Root    string    `json:"root"`
	Used    int64     `json:"used"`
	Free    int64     `json:"free"`
	Total   int64     `json:"total"`
	Scanned time.Time `json:"scanned"`
}

var storageCache struct {
	sync.Mutex
	info    StorageInfo
	running bool
}

func (p *PBX) Storage() StorageInfo {
	root := p.RecordingsRoot()
	storageCache.Lock()
	info := storageCache.info
	stale := info.Root != root || time.Since(info.Scanned) > 5*time.Minute
	if stale && !storageCache.running {
		storageCache.running = true
		go func() {
			var used int64
			_ = filepath.Walk(root, func(_ string, fi os.FileInfo, err error) error {
				if err == nil && fi.Mode().IsRegular() {
					used += fi.Size()
				}
				return nil
			})
			ni := StorageInfo{Root: root, Used: used, Scanned: time.Now()}
			ni.Free, ni.Total = diskSpace(root)
			storageCache.Lock()
			storageCache.info = ni
			storageCache.running = false
			storageCache.Unlock()
		}()
	}
	storageCache.Unlock()
	if info.Root != root {
		info = StorageInfo{Root: root}
		info.Free, info.Total = diskSpace(root)
	}
	return info
}

func diskSpace(path string) (free, total int64) {
	for p := path; ; p = filepath.Dir(p) {
		var s syscall.Statfs_t
		if err := syscall.Statfs(p, &s); err == nil {
			return int64(s.Bavail) * int64(s.Bsize), int64(s.Blocks) * int64(s.Bsize)
		}
		if p == "/" || p == "." {
			return 0, 0
		}
	}
}
