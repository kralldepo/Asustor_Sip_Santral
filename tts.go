package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Voice is a Piper text-to-speech voice that can be downloaded on demand.
type Voice struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Lang  string `json:"lang"`
	Path  string `json:"-"`
	Size  int64  `json:"size"`
	State string `json:"state"` // none | downloading | ready | error
	Done  int64  `json:"done,omitempty"`
	Error string `json:"error,omitempty"`
}

const voiceBaseURL = "https://huggingface.co/rhasspy/piper-voices/resolve/v1.0.0/"

var voiceCatalog = []Voice{
	{ID: "tr_TR-fahrettin-medium", Name: "Türkçe · Fahrettin", Lang: "tr", Path: "tr/tr_TR/fahrettin/medium/", Size: 63201294},
	{ID: "tr_TR-fettah-medium", Name: "Türkçe · Fettah", Lang: "tr", Path: "tr/tr_TR/fettah/medium/", Size: 63201294},
	{ID: "tr_TR-dfki-medium", Name: "Türkçe · DFKI", Lang: "tr", Path: "tr/tr_TR/dfki/medium/", Size: 63201294},
	{ID: "de_DE-thorsten-medium", Name: "Deutsch · Thorsten", Lang: "de", Path: "de/de_DE/thorsten/medium/", Size: 63201294},
	{ID: "de_DE-kerstin-low", Name: "Deutsch · Kerstin", Lang: "de", Path: "de/de_DE/kerstin/low/", Size: 63104526},
	{ID: "en_US-amy-medium", Name: "English · Amy", Lang: "en", Path: "en/en_US/amy/medium/", Size: 63201294},
}

// TTS downloads voices and synthesizes announcements with Piper inside the
// root file system.
type TTS struct {
	pbx *PBX
	log *log.Logger

	mu        sync.Mutex
	progress  map[string]*Voice
	cache     map[string]ttsResult
	synthLock sync.Mutex
}

type ttsResult struct {
	wav  []byte
	made time.Time
}

func NewTTS(p *PBX, lg *log.Logger) *TTS {
	return &TTS{pbx: p, log: lg, progress: map[string]*Voice{}, cache: map[string]ttsResult{}}
}

func findVoice(id string) (Voice, bool) {
	for _, v := range voiceCatalog {
		if v.ID == id {
			return v, true
		}
	}
	return Voice{}, false
}

func (t *TTS) voiceFile(id string) string { return filepath.Join(t.pbx.voicesDir(), id+".onnx") }

// Available reports whether the Piper engine exists in the root file system.
func (t *TTS) Available() bool {
	if !canRunPBX {
		return false
	}
	p, err := t.pbx.rf.Path("/usr/bin/piper_exe")
	if err != nil {
		return false
	}
	_, err = os.Stat(p)
	return err == nil
}

func (t *TTS) Voices() []Voice {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]Voice, 0, len(voiceCatalog))
	for _, v := range voiceCatalog {
		if pr, ok := t.progress[v.ID]; ok && pr.State != "ready" {
			out = append(out, *pr)
			continue
		}
		if _, err := os.Stat(t.voiceFile(v.ID)); err == nil {
			v.State = "ready"
		} else {
			v.State = "none"
		}
		out = append(out, v)
	}
	return out
}

// Download fetches a voice model in the background.
func (t *TTS) Download(id string) error {
	v, ok := findVoice(id)
	if !ok {
		return errNotFound
	}
	t.mu.Lock()
	if pr, ok := t.progress[id]; ok && pr.State == "downloading" {
		t.mu.Unlock()
		return nil
	}
	pr := v
	pr.State = "downloading"
	t.progress[id] = &pr
	t.mu.Unlock()
	go func() {
		err := t.download(v)
		t.mu.Lock()
		defer t.mu.Unlock()
		if err != nil {
			t.progress[id].State = "error"
			t.progress[id].Error = err.Error()
			t.pbx.ev.Add("error", "Ses", v.Name+" indirilemedi: "+err.Error())
			return
		}
		t.progress[id].State = "ready"
		t.pbx.ev.Add("info", "Ses", v.Name+" sesi indirildi")
		t.pbx.syncVoices()
	}()
	return nil
}

func (t *TTS) download(v Voice) error {
	dir := t.pbx.voicesDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	client := &http.Client{Timeout: 30 * time.Minute}
	for _, suffix := range []string{".onnx.json", ".onnx"} {
		url := voiceBaseURL + v.Path + v.ID + suffix
		resp, err := client.Get(url)
		if err != nil {
			return err
		}
		if resp.StatusCode != 200 {
			resp.Body.Close()
			return fmt.Errorf("HTTP %d", resp.StatusCode)
		}
		part := filepath.Join(dir, v.ID+suffix+".part")
		f, err := os.Create(part)
		if err != nil {
			resp.Body.Close()
			return err
		}
		var n int64
		buf := make([]byte, 256<<10)
		for {
			k, rerr := resp.Body.Read(buf)
			if k > 0 {
				if _, werr := f.Write(buf[:k]); werr != nil {
					f.Close()
					resp.Body.Close()
					return werr
				}
				n += int64(k)
				if suffix == ".onnx" {
					t.mu.Lock()
					t.progress[v.ID].Done = n
					t.mu.Unlock()
				}
			}
			if rerr == io.EOF {
				break
			}
			if rerr != nil {
				f.Close()
				resp.Body.Close()
				return rerr
			}
		}
		resp.Body.Close()
		if err := f.Close(); err != nil {
			return err
		}
		if suffix == ".onnx" && n < 1<<20 {
			return fmt.Errorf("dosya eksik indi")
		}
		if err := os.Rename(part, filepath.Join(dir, v.ID+suffix)); err != nil {
			return err
		}
	}
	return nil
}

func (t *TTS) DeleteVoice(id string) error {
	if _, ok := findVoice(id); !ok {
		return errNotFound
	}
	for _, s := range []string{".onnx", ".onnx.json"} {
		_ = os.Remove(filepath.Join(t.pbx.voicesDir(), id+s))
		if p, err := t.pbx.rf.Path("/voices/" + id + s); err == nil {
			_ = os.Remove(p)
		}
	}
	t.mu.Lock()
	delete(t.progress, id)
	t.mu.Unlock()
	return nil
}

func ttsKey(text, voice string, speed float64) string {
	h := sha256.Sum256([]byte(voice + "|" + strconv.FormatFloat(speed, 'f', 2, 64) + "|" + text))
	return hex.EncodeToString(h[:12])
}

// Synthesize returns an 8 kHz telephone-ready WAV for the text.
func (t *TTS) Synthesize(ctx context.Context, text, voice string, speed float64) ([]byte, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, verr("Okunacak metni yazın")
	}
	if runeLen(text) > 1500 {
		return nil, verr("Metin en fazla 1500 karakter olabilir")
	}
	if speed < 0.6 || speed > 1.6 {
		speed = 1
	}
	if _, ok := findVoice(voice); !ok {
		return nil, verr("Ses seçin")
	}
	if !t.Available() {
		return nil, verr("Yazıdan seslendirme yalnız NAS üzerinde çalışır")
	}
	if _, err := os.Stat(t.voiceFile(voice)); err != nil {
		return nil, verr("Bu ses henüz indirilmedi")
	}
	key := ttsKey(text, voice, speed)
	t.mu.Lock()
	if r, ok := t.cache[key]; ok && time.Since(r.made) < 30*time.Minute {
		t.mu.Unlock()
		return r.wav, nil
	}
	t.mu.Unlock()

	t.synthLock.Lock() // the NAS has little RAM: one synthesis at a time
	defer t.synthLock.Unlock()
	t.pbx.syncVoices()
	tmpName := "tts-" + key + ".wav"
	outInside := "/tmp/" + tmpName
	outHost, err := t.pbx.rf.Path(outInside)
	if err != nil {
		return nil, err
	}
	defer os.Remove(outHost)
	cmd := t.pbx.rf.chrootCmd("/usr/bin/piper_exe", "-m", "/voices/"+voice+".onnx", "-f", outInside,
		"--length_scale", strconv.FormatFloat(1/speed, 'f', 3, 64))
	cmd.Stdin = strings.NewReader(strings.ReplaceAll(text, "\r", "") + "\n")
	var stderr bytes.Buffer
	cmd.Stdout = &stderr
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("Piper başlatılamadı: %w", err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			msg := strings.TrimSpace(stderr.String())
			if len(msg) > 300 {
				msg = msg[len(msg)-300:]
			}
			return nil, fmt.Errorf("seslendirme başarısız: %v %s", err, msg)
		}
	case <-time.After(3 * time.Minute):
		killGroup(cmd, syscall.SIGKILL)
		return nil, fmt.Errorf("seslendirme zaman aşımına uğradı")
	case <-ctx.Done():
		killGroup(cmd, syscall.SIGKILL)
		return nil, ctx.Err()
	}
	f, err := os.Open(outHost)
	if err != nil {
		return nil, fmt.Errorf("seslendirme çıktısı yok: %w", err)
	}
	pcm, err := ReadWAV(f)
	f.Close()
	if err != nil {
		return nil, err
	}
	pcm = PCM{Rate: pcm.Rate, Samples: append(append(Silence(150*time.Millisecond, pcm.Rate).Samples, pcm.Samples...), Silence(250*time.Millisecond, pcm.Rate).Samples...)}
	out, err := PrepareAnnouncement(pcm)
	if err != nil {
		return nil, err
	}
	wav := EncodeWAV16(out)
	t.mu.Lock()
	for k, r := range t.cache {
		if time.Since(r.made) > 30*time.Minute {
			delete(t.cache, k)
		}
	}
	t.cache[key] = ttsResult{wav: wav, made: time.Now()}
	t.mu.Unlock()
	return wav, nil
}
