//go:build e2e

// End-to-end test: SIP Santral (panel + Asterisk in its chroot) against a
// simulated provider/phone Asterisk, both in Docker.
//
// From the project root:
//
//	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o build/linux-arm64/sipsantral .
//	docker build -t ss-sim test/sim
//	go test -tags e2e -v -timeout 20m ./test/e2e
//
// KEEP=1 leaves the containers running afterwards.
package e2e

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const base = "http://127.0.0.1:19490"

type client struct {
	t *testing.T
	h *http.Client
}

func (c *client) do(method, path string, body any, out any) (int, []byte) {
	c.t.Helper()
	var rd io.Reader
	ct := ""
	switch b := body.(type) {
	case nil:
	case *multipartBody:
		rd, ct = bytes.NewReader(b.data), b.ct
	default:
		raw, _ := json.Marshal(b)
		rd, ct = bytes.NewReader(raw), "application/json"
	}
	req, _ := http.NewRequest(method, base+path, rd)
	req.Header.Set("X-Requested-With", "sipsantral")
	if ct != "" {
		req.Header.Set("Content-Type", ct)
	}
	resp, err := c.h.Do(req)
	if err != nil {
		c.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if out != nil && resp.StatusCode < 300 {
		if err := json.Unmarshal(data, out); err != nil {
			c.t.Fatalf("%s %s: json: %v (%s)", method, path, err, data)
		}
	}
	return resp.StatusCode, data
}

func (c *client) must(method, path string, body any, out any) {
	c.t.Helper()
	code, data := c.do(method, path, body, out)
	if code >= 300 {
		c.t.Fatalf("%s %s -> %d %s", method, path, code, data)
	}
}

type multipartBody struct {
	data []byte
	ct   string
}

func run(t *testing.T, name string, args ...string) string {
	t.Helper()
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v: %v\n%s", name, args, err, out)
	}
	return string(out)
}

func runQuiet(name string, args ...string) string {
	out, _ := exec.Command(name, args...).CombinedOutput()
	return string(out)
}

func waitFor(t *testing.T, what string, timeout time.Duration, f func() (bool, string)) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	last := ""
	for time.Now().Before(deadline) {
		ok, info := f()
		if ok {
			return
		}
		last = info
		time.Sleep(time.Second)
	}
	t.Fatalf("zaman aşımı: %s (son durum: %s)", what, last)
}

func toneWAV(freq float64, sec float64, rate int) []byte {
	n := int(sec * float64(rate))
	var buf bytes.Buffer
	buf.WriteString("RIFF")
	binary.Write(&buf, binary.LittleEndian, uint32(36+n*2))
	buf.WriteString("WAVEfmt ")
	for _, v := range []any{uint32(16), uint16(1), uint16(1), uint32(rate), uint32(rate * 2), uint16(2), uint16(16)} {
		binary.Write(&buf, binary.LittleEndian, v)
	}
	buf.WriteString("data")
	binary.Write(&buf, binary.LittleEndian, uint32(n*2))
	for i := 0; i < n; i++ {
		v := int16(12000 * math.Sin(2*math.Pi*freq*float64(i)/float64(rate)))
		binary.Write(&buf, binary.LittleEndian, v)
	}
	return buf.Bytes()
}

type callRec struct {
	ID         string `json:"id"`
	Result     string `json:"result"`
	AnsweredBy string `json:"answeredBy"`
	Caller     string `json:"caller"`
	Rec        string `json:"rec"`
	RecSec     int    `json:"recSec"`
	RecBytes   int64  `json:"recBytes"`
	DurSec     int    `json:"durSec"`
	TalkSec    int    `json:"talkSec"`
	LineName   string `json:"lineName"`
}

type callsResp struct {
	Items []callRec `json:"items"`
	Total int       `json:"total"`
	Live  []struct {
		State string `json:"state"`
		By    string `json:"by"`
	} `json:"live"`
}

func TestEndToEnd(t *testing.T) {
	root, _ := filepath.Abs("../..")
	bin := filepath.Join(root, "build/linux-arm64/sipsantral")
	rootfs := filepath.Join(root, "build/rootfs-arm64.tar.gz")
	for _, f := range []string{bin, rootfs} {
		if _, err := os.Stat(f); err != nil {
			t.Fatalf("eksik: %s", f)
		}
	}
	runQuiet("docker", "rm", "-f", "ss-santral", "ss-sim")
	runQuiet("docker", "network", "create", "sstest")
	if os.Getenv("KEEP") == "" {
		t.Cleanup(func() { runQuiet("docker", "rm", "-f", "ss-santral", "ss-sim") })
	}
	run(t, "docker", "run", "-d", "--name", "ss-sim", "--hostname", "sim", "--network", "sstest", "ss-sim")
	args := []string{"run", "-d", "--name", "ss-santral", "--hostname", "santral", "--privileged", "--network", "sstest",
		"-p", "127.0.0.1:19490:9490", "-v", bin + ":/app/bin/sipsantral:ro", "-v", rootfs + ":/app/rootfs.tar.gz:ro"}
	extra := ""
	if os.Getenv("SS_UIDEV") != "" { // serve the UI from the working tree (live edits)
		args = append(args, "-v", filepath.Join(root, "ui")+":/app/ui-dev:ro")
		extra = " -ui-dir /app/ui-dev"
	}
	args = append(args, "alpine:3.22", "sh", "-c", "mkdir -p /volume1/SipKayitlari && exec /app/bin/sipsantral -pkg /app -http -listen :9490"+extra)
	run(t, "docker", args...)

	jar, _ := cookiejar.New(nil)
	c := &client{t: t, h: &http.Client{Jar: jar, Timeout: 5 * time.Minute}}
	waitFor(t, "panel", 60*time.Second, func() (bool, string) {
		resp, err := http.Get(base + "/api/status")
		if err != nil {
			return false, err.Error()
		}
		resp.Body.Close()
		return resp.StatusCode == 200, resp.Status
	})
	c.must("POST", "/api/setup", map[string]string{"password": "testparola1"}, nil)
	c.must("POST", "/api/extensions", map[string]string{"num": "101", "name": "Test telefon", "password": "Ext101Pass1"}, nil)

	// announcement upload (16 kHz tone, converted to 8 kHz by the panel)
	var mb bytes.Buffer
	mw := multipart.NewWriter(&mb)
	mw.WriteField("name", "Test anons")
	mw.WriteField("source", "file")
	fw, _ := mw.CreateFormFile("audio", "test.wav")
	fw.Write(toneWAV(700, 1.5, 16000))
	mw.Close()
	var ann struct {
		ID    string `json:"id"`
		DurMs int    `json:"durMs"`
	}
	c.must("POST", "/api/announcements", &multipartBody{mb.Bytes(), mw.FormDataContentType()}, &ann)
	if ann.DurMs < 1300 || ann.DurMs > 1700 {
		t.Fatalf("anons süresi beklenmedik: %d ms", ann.DurMs)
	}

	line := map[string]any{"name": "Test hattı", "number": "+49 30 1234", "server": "sim", "port": 5060, "username": "line1",
		"password": "line1pass", "mode": "ring", "exts": []string{"101"}, "record": true, "cidPrefix": true, "ringSec": 20}
	var created struct {
		ID string `json:"id"`
	}
	c.must("POST", "/api/lines", line, &created)
	lineID := created.ID

	type dash struct {
		PBX struct {
			State string `json:"state"`
		} `json:"pbx"`
		Lines []struct {
			ID     string `json:"id"`
			Status struct {
				State  string `json:"state"`
				Detail string `json:"detail"`
			} `json:"status"`
		} `json:"lines"`
		Exts []struct {
			Num    string `json:"num"`
			Online bool   `json:"online"`
		} `json:"exts"`
		Live []struct {
			State string `json:"state"`
		} `json:"live"`
	}
	waitFor(t, "hat kaydı ve dahili bağlantısı", 90*time.Second, func() (bool, string) {
		var d dash
		c.must("GET", "/api/dashboard", nil, &d)
		ok := d.PBX.State == "running" && len(d.Lines) == 1 && d.Lines[0].Status.State == "registered" && len(d.Exts) == 1 && d.Exts[0].Online
		return ok, fmt.Sprintf("%+v", d)
	})
	t.Log("hat kayıtlı, dahili 101 bağlı")

	known := map[string]bool{}
	nextCall := func(timeout time.Duration) callRec {
		t.Helper()
		var got callRec
		waitFor(t, "yeni arama kaydı", timeout, func() (bool, string) {
			var cr callsResp
			c.must("GET", "/api/calls?range=today&limit=20", nil, &cr)
			for _, it := range cr.Items {
				if !known[it.ID] {
					known[it.ID] = true
					got = it
					return true, ""
				}
			}
			return false, fmt.Sprintf("toplam %d, canlı %d", cr.Total, len(cr.Live))
		})
		return got
	}
	originate := func(account string) {
		run(t, "docker", "exec", "ss-sim", "asterisk", "-rx", "channel originate PJSIP/"+account+" extension call@caller")
	}
	setLine := func(changes map[string]any) {
		for k, v := range changes {
			line[k] = v
		}
		c.must("PUT", "/api/lines/"+lineID, line, nil)
		time.Sleep(2 * time.Second) // dialplan reload
	}
	checkRecording := func(r callRec) {
		t.Helper()
		if r.Rec == "" {
			t.Fatalf("kayıt dosyası yok: %+v", r)
		}
		out := run(t, "docker", "exec", "ss-santral", "ls", "-la", "/volume1/SipKayitlari/"+r.Rec)
		t.Logf("kayıt: %s", strings.TrimSpace(out))
		req, _ := http.NewRequest("GET", base+"/api/calls/"+r.ID+"/audio", nil)
		resp, err := c.h.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		data, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "audio/mpeg" || len(data) < 2000 {
			t.Fatalf("ses dosyası okunamadı: %d %s %d bayt", resp.StatusCode, resp.Header.Get("Content-Type"), len(data))
		}
		if !(bytes.HasPrefix(data, []byte("ID3")) || (data[0] == 0xFF && data[1]&0xE0 == 0xE0)) {
			t.Fatalf("MP3 değil: % x", data[:4])
		}
	}

	t.Run("çaldır-cevaplandı", func(t *testing.T) {
		originate("line1")
		waitFor(t, "canlı arama", 20*time.Second, func() (bool, string) {
			var d dash
			c.must("GET", "/api/dashboard", nil, &d)
			for _, l := range d.Live {
				if l.State == "answered" {
					return true, ""
				}
			}
			return false, fmt.Sprintf("%+v", d.Live)
		})
		r := nextCall(60 * time.Second)
		t.Logf("%+v", r)
		if r.Result != "answered" || r.AnsweredBy != "101" || r.Caller != "+4917012345678" {
			t.Fatalf("beklenmeyen sonuç: %+v", r)
		}
		if r.TalkSec < 8 || r.RecSec < 8 {
			t.Fatalf("konuşma/kayıt süresi kısa: %+v", r)
		}
		checkRecording(r)
	})

	t.Run("anons+çaldır", func(t *testing.T) {
		setLine(map[string]any{"mode": "annring", "annId": ann.ID})
		originate("line1")
		r := nextCall(60 * time.Second)
		t.Logf("%+v", r)
		if r.Result != "answered" || r.AnsweredBy != "101" {
			t.Fatalf("beklenmeyen sonuç: %+v", r)
		}
		checkRecording(r)
	})

	t.Run("sadece-anons+mesaj", func(t *testing.T) {
		setLine(map[string]any{"mode": "ann", "annId": ann.ID, "after": "vm"})
		originate("line1")
		r := nextCall(60 * time.Second)
		t.Logf("%+v", r)
		if r.Result != "voicemail" {
			t.Fatalf("sesli mesaj bekleniyordu: %+v", r)
		}
		checkRecording(r)
	})

	t.Run("sadece-anons+kapat", func(t *testing.T) {
		setLine(map[string]any{"mode": "ann", "annId": ann.ID, "after": "hang"})
		originate("line1")
		r := nextCall(60 * time.Second)
		t.Logf("%+v", r)
		if r.Result != "announcement" || r.DurSec < 2 {
			t.Fatalf("anons (en az 2 sn) bekleniyordu: %+v", r)
		}
	})

	t.Run("cevapsız", func(t *testing.T) {
		run(t, "docker", "exec", "ss-sim", "asterisk", "-rx", "database put test noanswer 1")
		defer run(t, "docker", "exec", "ss-sim", "asterisk", "-rx", "database del test noanswer")
		setLine(map[string]any{"mode": "ring", "ringSec": 5, "noAnswer": "hang"})
		originate("line1")
		r := nextCall(60 * time.Second)
		t.Logf("%+v", r)
		if r.Result != "missed" || r.Rec != "" {
			t.Fatalf("kayıtsız cevapsız arama bekleniyordu: %+v", r)
		}
	})

	t.Run("cevapsız-mesaj", func(t *testing.T) {
		run(t, "docker", "exec", "ss-sim", "asterisk", "-rx", "database put test noanswer 1")
		defer run(t, "docker", "exec", "ss-sim", "asterisk", "-rx", "database del test noanswer")
		setLine(map[string]any{"mode": "ring", "ringSec": 5, "noAnswer": "vm", "noAnsAnnId": ann.ID})
		originate("line1")
		r := nextCall(60 * time.Second)
		t.Logf("%+v", r)
		if r.Result != "voicemail" {
			t.Fatalf("sesli mesaj bekleniyordu: %+v", r)
		}
		checkRecording(r)
	})

	t.Run("çalıp-meşgul", func(t *testing.T) {
		setLine(map[string]any{"mode": "busy", "ringSec": 5, "exts": []string{}})
		originate("line1")
		r := nextCall(60 * time.Second)
		t.Logf("%+v", r)
		if r.Result != "busy" || r.Rec != "" || r.DurSec < 4 || r.DurSec > 8 {
			t.Fatalf("5 sn çalıp meşgul bekleniyordu: %+v", r)
		}
	})

	t.Run("cevapsız-meşgul-ver", func(t *testing.T) {
		run(t, "docker", "exec", "ss-sim", "asterisk", "-rx", "database put test noanswer 1")
		defer run(t, "docker", "exec", "ss-sim", "asterisk", "-rx", "database del test noanswer")
		setLine(map[string]any{"mode": "ring", "exts": []string{"101"}, "ringSec": 5, "noAnswer": "busy"})
		originate("line1")
		r := nextCall(60 * time.Second)
		t.Logf("%+v", r)
		if r.Result != "missed" || r.DurSec < 4 || r.DurSec > 9 {
			t.Fatalf("cevapsız (meşgul verilmiş) bekleniyordu: %+v", r)
		}
	})

	t.Run("toplu-işlem", func(t *testing.T) {
		text := "Toplu 1;+49 1;sim;topla1;x1\nToplu 2;+49 2;sim;topla2;x2\nToplu 3;+49 3;sim;topla3;x3"
		var imp struct {
			Added int `json:"added"`
		}
		c.must("POST", "/api/lines/import", map[string]any{"text": text}, &imp)
		if imp.Added != 3 {
			t.Fatalf("içe aktarılan %d", imp.Added)
		}
		type lineInfo struct {
			ID      string   `json:"id"`
			Name    string   `json:"name"`
			Mode    string   `json:"mode"`
			Exts    []string `json:"exts"`
			Enabled bool     `json:"enabled"`
			RingSec int      `json:"ringSec"`
		}
		var ls struct {
			Lines []lineInfo `json:"lines"`
		}
		get := func() map[string]lineInfo {
			c.must("GET", "/api/lines", nil, &ls)
			m := map[string]lineInfo{}
			for _, l := range ls.Lines {
				if strings.HasPrefix(l.Name, "Toplu") {
					m[l.ID] = l
				}
			}
			return m
		}
		var ids []string
		for id, l := range get() {
			if l.Mode != "busy" || len(l.Exts) != 0 && l.Mode != "busy" {
				t.Fatalf("içe aktarılan hat varsayılanı: %+v", l)
			}
			ids = append(ids, id)
		}
		// anonsuz anonslu mod: hiçbiri değişmemeli
		code, body := c.do("POST", "/api/lines/bulk", map[string]any{"ids": ids, "action": "update", "changes": map[string]any{"mode": "annring", "annId": ""}}, nil)
		if code != 400 || !strings.Contains(string(body), "hat daha") {
			t.Fatalf("geçersiz toplu güncelleme kabul edildi: %d %s", code, body)
		}
		c.must("POST", "/api/lines/bulk", map[string]any{"ids": ids, "action": "update", "changes": map[string]any{"mode": "ring", "exts": []string{"101"}, "ringSec": 25}}, nil)
		for _, l := range get() {
			if l.Mode != "ring" || len(l.Exts) != 1 || l.RingSec != 25 {
				t.Fatalf("toplu güncelleme uygulanmadı: %+v", l)
			}
		}
		c.must("POST", "/api/lines/bulk", map[string]any{"ids": ids, "action": "disable"}, nil)
		for _, l := range get() {
			if l.Enabled {
				t.Fatalf("toplu kapatma uygulanmadı: %+v", l)
			}
		}
		c.must("POST", "/api/lines/bulk", map[string]any{"ids": ids, "action": "delete"}, nil)
		if n := len(get()); n != 0 {
			t.Fatalf("toplu silmeden sonra %d hat kaldı", n)
		}
	})

	t.Run("hat-testi-ve-hatalı-şifre", func(t *testing.T) {
		var st struct {
			State string `json:"state"`
		}
		c.must("POST", "/api/lines/"+lineID+"/test", nil, &st)
		if st.State != "registered" {
			t.Fatalf("test sonucu: %+v", st)
		}
		bad := map[string]any{"name": "Yanlış şifre", "server": "sim", "username": "line2", "password": "yanlis",
			"mode": "ring", "exts": []string{"101"}, "record": true}
		var b struct {
			ID string `json:"id"`
		}
		c.must("POST", "/api/lines", bad, &b)
		waitFor(t, "hatalı şifre durumu", 60*time.Second, func() (bool, string) {
			var d dash
			c.must("GET", "/api/dashboard", nil, &d)
			for _, l := range d.Lines {
				if l.ID == b.ID {
					return l.Status.State == "failed", fmt.Sprintf("%+v", l.Status)
				}
			}
			return false, "hat yok"
		})
		c.must("DELETE", "/api/lines/"+b.ID, nil, nil)
	})

	t.Run("asterisk-uyarıları", func(t *testing.T) {
		out := runQuiet("docker", "exec", "ss-santral", "sh", "-c", "grep -E 'WARNING|ERROR' /app/rootfs/var/log/asterisk/messages.log || true")
		var bad []string
		for _, l := range strings.Split(out, "\n") {
			if l == "" || strings.Contains(l, "nameservers") || strings.Contains(l, "not reloadable") {
				continue
			}
			t.Log(l)
			if strings.Contains(l, "does not exist") || strings.Contains(l, "Unable to") || strings.Contains(l, "Failed to") {
				bad = append(bad, l)
			}
		}
		if len(bad) > 0 {
			t.Fatalf("%d önemli Asterisk uyarısı", len(bad))
		}
	})

	t.Run("csv-ve-zip", func(t *testing.T) {
		code, csvData := c.do("GET", "/api/calls/export.csv?range=today", nil, nil)
		if code != 200 || strings.Count(string(csvData), "\n") < 6 {
			t.Fatalf("CSV: %d\n%s", code, csvData)
		}
		var cr callsResp
		c.must("GET", "/api/calls?range=today&limit=50", nil, &cr)
		var ids []string
		for _, it := range cr.Items {
			if it.Rec != "" {
				ids = append(ids, it.ID)
			}
		}
		code, zdata := c.do("POST", "/api/calls/zip", map[string]any{"ids": ids}, nil)
		if code != 200 {
			t.Fatalf("ZIP: %d %s", code, zdata)
		}
		zr, err := zip.NewReader(bytes.NewReader(zdata), int64(len(zdata)))
		if err != nil || len(zr.File) != len(ids) {
			t.Fatalf("ZIP içeriği: %v %d/%d", err, len(zr.File), len(ids))
		}
		t.Logf("ZIP: %d kayıt", len(zr.File))
	})
}
