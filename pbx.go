package main

import (
	"bufio"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

type LineStatus struct {
	State  string    `json:"state"` // registered | failed | trying | disabled | unknown
	Detail string    `json:"detail,omitempty"`
	Since  time.Time `json:"since"`
}

type ExtStatus struct {
	Online   bool      `json:"online"`
	UA       string    `json:"ua,omitempty"`
	Addr     string    `json:"addr,omitempty"`
	RTTms    int       `json:"rttMs,omitempty"`
	Since    time.Time `json:"since"`
	LastSeen time.Time `json:"lastSeen,omitempty"`
}

type LiveCall struct {
	ID         string    `json:"id"`
	LineID     string    `json:"lineId"`
	Caller     string    `json:"caller"`
	CallerName string    `json:"callerName,omitempty"`
	Mode       string    `json:"mode"`
	State      string    `json:"state"` // started | announcement | ringing | answered | noanswer | voicemail
	Targets    string    `json:"targets,omitempty"`
	By         string    `json:"by,omitempty"`
	Start      time.Time `json:"start"`
	AnsweredAt time.Time `json:"answeredAt,omitempty"`
	Recording  bool      `json:"recording"`
}

type failInfo struct {
	count int
	first time.Time
}

// PBX runs Asterisk inside the root file system and keeps track of lines,
// phones and calls through AMI.
type PBX struct {
	pkgDir, dataDir string
	store           *Store
	calls           *CallLog
	ev              *EventLog
	log             *log.Logger
	rf              *Rootfs
	amiPort         int
	enabled         bool

	mu           sync.Mutex
	state        string
	stateMsg     string
	astStart     time.Time
	astVersion   string
	ami          *AMI
	lineSt       map[string]*LineStatus
	extSt        map[string]*ExtStatus
	live         map[string]*LiveCall
	regCause     map[string]string
	rt           astRuntime
	lastFiles    map[string]string
	transportKey string
	pendRestart  bool
	pendReload   bool
	fails        map[string]*failInfo
	blocked      map[string]time.Time
	extIPChecked time.Time

	reloadCh  chan struct{}
	restartCh chan struct{}
	statusCh  chan struct{}
	recJobs   chan recJob
}

var errRestart = errors.New("yeniden başlatma")

func NewPBX(pkgDir, dataDir string, store *Store, calls *CallLog, ev *EventLog, lg *log.Logger, enabled bool) *PBX {
	return &PBX{
		pkgDir: pkgDir, dataDir: dataDir, store: store, calls: calls, ev: ev, log: lg, enabled: enabled,
		rf:       &Rootfs{Dir: filepath.Join(pkgDir, "rootfs"), Tarball: filepath.Join(pkgDir, "rootfs.tar.gz"), log: lg},
		amiPort:  5038,
		state:    "stopped",
		lineSt:   map[string]*LineStatus{},
		extSt:    map[string]*ExtStatus{},
		live:     map[string]*LiveCall{},
		regCause: map[string]string{},
		fails:    map[string]*failInfo{},
		blocked:  map[string]time.Time{},
		reloadCh: make(chan struct{}, 1), restartCh: make(chan struct{}, 1),
		statusCh: make(chan struct{}, 1), recJobs: make(chan recJob, 512),
	}
}

func (p *PBX) setState(state, msg string) {
	p.mu.Lock()
	changed := p.state != state
	p.state, p.stateMsg = state, msg
	p.mu.Unlock()
	if changed {
		p.log.Printf("santral durumu: %s %s", state, msg)
	}
}

// ---------------------------------------------------------------------------
// Main loop
// ---------------------------------------------------------------------------

func (p *PBX) Run(ctx context.Context) {
	go p.recorderLoop(ctx)
	go p.maintenanceLoop(ctx)
	go p.configLoop(ctx)
	if !p.enabled || !canRunPBX {
		msg := "Asterisk yalnız NAS üzerinde (Linux) çalışır"
		if !p.enabled {
			msg = "Asterisk kapalı (-no-pbx)"
		}
		p.setState("disabled", msg)
		<-ctx.Done()
		return
	}
	if os.Geteuid() != 0 {
		p.setState("error", "Uygulama root yetkisiyle çalışmalı")
		<-ctx.Done()
		return
	}
	backoff := 3 * time.Second
	for ctx.Err() == nil {
		started := time.Now()
		err := p.runOnce(ctx)
		if ctx.Err() != nil {
			break
		}
		if errors.Is(err, errRestart) {
			backoff = 3 * time.Second
			continue
		}
		if time.Since(started) > 5*time.Minute {
			backoff = 3 * time.Second
		}
		msg := "bilinmeyen hata"
		if err != nil {
			msg = err.Error()
		}
		p.setState("error", msg)
		p.ev.Add("error", "Santral", "Asterisk durdu: "+msg+" — yeniden başlatılıyor")
		select {
		case <-ctx.Done():
		case <-time.After(backoff):
		case <-p.restartCh:
		}
		if backoff < time.Minute {
			backoff *= 2
		}
	}
	unmountAll(p.rf.Dir, p.log)
	p.setState("stopped", "")
}

func (p *PBX) runOnce(ctx context.Context) error {
	p.setState("preparing", "Asterisk ortamı hazırlanıyor")
	if err := p.rf.Ensure(func(s string) { p.setState("preparing", s) }); err != nil {
		return err
	}
	if err := p.prepareRootfs(); err != nil {
		return fmt.Errorf("ortam hazırlanamadı: %w", err)
	}
	p.refreshRuntime(ctx, true)
	if _, err := p.writeConfig(); err != nil {
		return fmt.Errorf("ayarlar yazılamadı: %w", err)
	}
	if err := p.rf.mountRuntime(); err != nil {
		return err
	}
	p.processOrphans()

	p.setState("starting", "Asterisk başlatılıyor")
	cmd := p.rf.chrootCmd("/usr/sbin/asterisk", "-f", "-n", "-U", "asterisk", "-G", "asterisk", "-C", "/etc/asterisk/asterisk.conf")
	out, _ := cmd.StdoutPipe()
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		unmountAll(p.rf.Dir, p.log)
		return fmt.Errorf("Asterisk başlatılamadı: %w", err)
	}
	go p.pipeLog(out)
	waitCh := make(chan error, 1)
	go func() { waitCh <- cmd.Wait() }()

	ami, err := p.connectAMI(ctx, waitCh)
	if err != nil {
		p.stopAsterisk(cmd, waitCh)
		return err
	}
	p.onAMIConnected(ami)
	p.ev.Add("info", "Santral", "Asterisk çalışıyor")
	for {
		select {
		case err := <-waitCh:
			p.dropAMI()
			unmountAll(p.rf.Dir, p.log)
			p.abandonLive()
			if err == nil {
				err = errors.New("beklenmedik çıkış")
			}
			return fmt.Errorf("Asterisk kapandı: %v", err)
		case <-ctx.Done():
			p.stopAsterisk(cmd, waitCh)
			return nil
		case <-p.restartCh:
			p.setState("starting", "Asterisk yeniden başlatılıyor")
			p.stopAsterisk(cmd, waitCh)
			return errRestart
		case <-ami.Done():
			p.dropAMI()
			p.log.Printf("AMI bağlantısı koptu, yeniden bağlanılıyor")
			ami, err = p.connectAMI(ctx, waitCh)
			if err != nil {
				p.stopAsterisk(cmd, waitCh)
				return err
			}
			p.onAMIConnected(ami)
		}
	}
}

func (p *PBX) pipeLog(r io.Reader) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	n := 0
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		if n < 2000 {
			p.log.Printf("asterisk: %s", line)
			n++
		}
	}
}

func (p *PBX) connectAMI(ctx context.Context, waitCh chan error) (*AMI, error) {
	deadline := time.Now().Add(60 * time.Second)
	for {
		a := NewAMI(fmt.Sprintf("127.0.0.1:%d", p.amiPort), "santral", p.store.AMISecret(), p.log)
		cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		err := a.Connect(cctx)
		cancel()
		if err == nil {
			return a, nil
		}
		select {
		case werr := <-waitCh:
			waitCh <- werr // let the caller see the exit too
			return nil, fmt.Errorf("Asterisk açılırken kapandı: %v", werr)
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Second):
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("AMI bağlantısı kurulamadı: %v", err)
		}
	}
}

func (p *PBX) onAMIConnected(a *AMI) {
	// AMI accepts logins before every module is loaded.
	if _, err := a.Command("core waitfullybooted"); err != nil {
		p.log.Printf("Asterisk açılışı beklenemedi: %v", err)
	}
	p.mu.Lock()
	p.ami = a
	p.mu.Unlock()
	go p.eventLoop(a)
	go p.statusLoop(a)
	if v, err := a.Action("CoreSettings", nil, 5*time.Second); err == nil {
		p.mu.Lock()
		p.astVersion = v.Get("AsteriskVersion")
		p.mu.Unlock()
	}
	p.applyDebug(a, p.store.Settings().SIPDebug)
	p.setState("running", "")
	p.mu.Lock()
	p.astStart = time.Now()
	p.mu.Unlock()
	p.kickStatus()
	p.RequestReload() // apply changes made while Asterisk was starting
}

func (p *PBX) dropAMI() {
	p.mu.Lock()
	if p.ami != nil {
		p.ami.Close()
	}
	p.ami = nil
	p.mu.Unlock()
}

func (p *PBX) currentAMI() *AMI {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.ami
}

func (p *PBX) stopAsterisk(cmd *exec.Cmd, waitCh chan error) {
	p.dropAMI()
	killGroup(cmd, syscall.SIGTERM)
	select {
	case <-waitCh:
	case <-time.After(10 * time.Second):
		killGroup(cmd, syscall.SIGKILL)
		select {
		case <-waitCh:
		case <-time.After(5 * time.Second):
		}
	}
	unmountAll(p.rf.Dir, p.log)
	p.abandonLive()
}

// abandonLive finishes calls whose end event never arrived.
func (p *PBX) abandonLive() {
	p.mu.Lock()
	var left []*LiveCall
	for id, lc := range p.live {
		left = append(left, lc)
		delete(p.live, id)
	}
	p.mu.Unlock()
	for _, lc := range left {
		rec := p.baseRecord(lc.ID, lc.LineID, lc.Mode, lc.Caller, lc.CallerName, lc.Start, time.Now())
		rec.Result = "interrupted"
		if lc.By != "" {
			rec.Result = "answered"
			rec.AnsweredBy = lc.By
			rec.AnsweredByName = p.extName(lc.By)
		}
		_ = p.calls.Add(rec)
	}
}

// ---------------------------------------------------------------------------
// Root file system preparation
// ---------------------------------------------------------------------------

func (p *PBX) soundsDir() string { return filepath.Join(p.dataDir, "sounds") }
func (p *PBX) voicesDir() string { return filepath.Join(p.dataDir, "voices") }

func (p *PBX) prepareRootfs() error {
	rf := p.rf
	for _, d := range []string{"/etc/asterisk", "/var/lib/asterisk", "/var/lib/asterisk/sounds/santral", "/var/spool/asterisk",
		"/var/spool/asterisk/monitor", "/var/log/asterisk", "/run/asterisk", "/santral"} {
		if _, err := rf.MkdirOwned(d, 0o755); err != nil {
			return err
		}
	}
	if p, err := rf.Path("/voices"); err == nil {
		_ = os.MkdirAll(p, 0o755)
	}
	rf.chownTree("/var/lib/asterisk")
	rf.chownTree("/var/spool/asterisk")
	rf.chownTree("/var/log/asterisk")
	rf.chownTree("/run/asterisk")
	script := "#!/bin/sh\n# MixMonitor bitince çağrılır: kaydın tamamlandığını işaretler.\n[ -n \"$1\" ] && : > \"/var/spool/asterisk/monitor/$1.done\"\nexit 0\n"
	if err := rf.WriteFile("/santral/recdone.sh", []byte(script), 0o755, false); err != nil {
		return err
	}
	p.copyHostFiles()
	if err := p.syncSounds(); err != nil {
		return err
	}
	p.syncVoices()
	return nil
}

// copyHostFiles gives the root file system the NAS's time zone and DNS
// settings (needed to resolve provider host names).
func (p *PBX) copyHostFiles() {
	for _, f := range []string{"/etc/localtime", "/etc/resolv.conf", "/etc/hosts"} {
		if b, err := os.ReadFile(f); err == nil && len(b) > 0 {
			_ = p.rf.WriteFile(f, b, 0o644, false)
		}
	}
}

// syncSounds links every announcement (plus the beep) into Asterisk's sound
// folder and removes stale ones.
func (p *PBX) syncSounds() error {
	dir, err := p.rf.Path("/var/lib/asterisk/sounds/santral")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	beep := EncodeWAV16(PCM{Rate: 8000, Samples: append(Silence(80*time.Millisecond, 8000).Samples, Tone(1000, 400*time.Millisecond, 8000, 0.5).Samples...)})
	if err := writeFileAtomic(filepath.Join(dir, "beep.wav"), beep, 0o644); err != nil {
		return err
	}
	want := map[string]bool{"beep.wav": true}
	for _, a := range p.store.Announcements() {
		name := a.ID + ".wav"
		want[name] = true
		src := filepath.Join(p.soundsDir(), name)
		dst := filepath.Join(dir, name)
		ss, err := os.Stat(src)
		if err != nil {
			continue
		}
		if ds, err := os.Stat(dst); err == nil && os.SameFile(ss, ds) {
			continue
		}
		if err := linkOrCopy(src, dst); err != nil {
			p.log.Printf("anons bağlanamadı %s: %v", name, err)
		}
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if !want[e.Name()] {
			_ = os.Remove(filepath.Join(dir, e.Name()))
		}
	}
	return nil
}

func (p *PBX) syncVoices() {
	dir, err := p.rf.Path("/voices")
	if err != nil {
		return
	}
	_ = os.MkdirAll(dir, 0o755)
	entries, _ := os.ReadDir(p.voicesDir())
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".part") || e.IsDir() {
			continue
		}
		src := filepath.Join(p.voicesDir(), e.Name())
		dst := filepath.Join(dir, e.Name())
		ss, err := os.Stat(src)
		if err != nil {
			continue
		}
		if ds, err := os.Stat(dst); err == nil && os.SameFile(ss, ds) {
			continue
		}
		_ = linkOrCopy(src, dst)
	}
}

// ---------------------------------------------------------------------------
// Configuration
// ---------------------------------------------------------------------------

func (p *PBX) refreshRuntime(ctx context.Context, force bool) bool {
	st := p.store.Settings()
	var nets []string
	if st.LocalNetMode == "manual" {
		nets = st.LocalNets
	} else {
		nets = detectLocalNets()
	}
	ext := ""
	switch st.ExtIPMode {
	case "manual":
		ext = st.ExtIP
	case "auto":
		p.mu.Lock()
		ext = p.rt.ExtIP
		due := force || time.Since(p.extIPChecked) > 10*time.Minute
		p.mu.Unlock()
		if due {
			cctx, cancel := context.WithTimeout(ctx, 6*time.Second)
			ip, err := stunPublicIP(cctx)
			cancel()
			p.mu.Lock()
			p.extIPChecked = time.Now()
			p.mu.Unlock()
			if err == nil {
				ext = ip
			} else if ext == "" {
				p.log.Printf("dış IP bulunamadı: %v", err)
			}
		}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	var blocked []string
	for ip, until := range p.blocked {
		if time.Now().Before(until) {
			blocked = append(blocked, ip)
		}
	}
	sort.Strings(blocked)
	newRT := astRuntime{LocalNets: nets, ExtIP: ext, Blocked: blocked, AMIPort: p.amiPort}
	changed := fmt.Sprint(newRT) != fmt.Sprint(p.rt)
	if changed && p.rt.ExtIP != "" && ext != p.rt.ExtIP {
		p.ev.Add("info", "Ağ", fmt.Sprintf("Dış IP değişti: %s", ext))
	}
	p.rt = newRT
	return changed
}

// writeConfig renders and writes Asterisk's files; it returns the names of
// the files whose content changed.
func (p *PBX) writeConfig() (map[string]bool, error) {
	cfg := p.store.Snapshot()
	p.mu.Lock()
	rt := p.rt
	p.mu.Unlock()
	files := renderAsteriskConfig(cfg, rt)
	changed := map[string]bool{}
	for name, content := range files {
		p.mu.Lock()
		old := p.lastFiles[name]
		p.mu.Unlock()
		if old == content {
			continue
		}
		if err := p.rf.WriteFile("/etc/asterisk/"+name, []byte(content), 0o640, true); err != nil {
			return nil, err
		}
		changed[name] = true
	}
	p.mu.Lock()
	p.lastFiles = files
	st := cfg.Settings
	p.transportKey = fmt.Sprintf("%d|%v|%v", st.SIPPort, st.UDP, st.TCP)
	p.mu.Unlock()
	return changed, nil
}

// RequestReload is called whenever the configuration changes.
func (p *PBX) RequestReload() {
	select {
	case p.reloadCh <- struct{}{}:
	default:
	}
}

func (p *PBX) configLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-p.reloadCh:
		}
		time.Sleep(400 * time.Millisecond) // collect bursts of changes
		select {
		case <-p.reloadCh:
		default:
		}
		p.applyConfig(ctx)
	}
}

func (p *PBX) applyConfig(ctx context.Context) {
	p.mu.Lock()
	state := p.state
	oldKey := p.transportKey
	p.mu.Unlock()
	if state != "running" {
		return // written at the next start
	}
	p.refreshRuntime(ctx, false)
	p.copyHostFiles()
	if err := p.syncSounds(); err != nil {
		p.log.Printf("sesler eşitlenemedi: %v", err)
	}
	p.syncVoices()
	changed, err := p.writeConfig()
	if err != nil {
		p.ev.Add("error", "Santral", "Ayarlar yazılamadı: "+err.Error())
		return
	}
	p.mu.Lock()
	newKey := p.transportKey
	p.mu.Unlock()
	needRestart := oldKey != newKey
	for _, f := range []string{"asterisk.conf", "modules.conf", "manager.conf", "rtp.conf", "indications.conf"} {
		if changed[f] {
			needRestart = true
		}
	}
	if needRestart {
		p.scheduleRestart()
		return
	}
	a := p.currentAMI()
	if a == nil {
		return
	}
	run := func(cmd string) {
		if _, err := a.Command(cmd); err != nil {
			p.log.Printf("%s: %v", cmd, err)
		}
	}
	if changed["pjsip.conf"] {
		run("module reload res_pjsip.so")
	}
	if changed["extensions.conf"] {
		run("dialplan reload")
	}
	if changed["logger.conf"] {
		run("logger reload")
	}
	p.kickStatus()
}

func (p *PBX) scheduleRestart() {
	p.mu.Lock()
	busy := len(p.live) > 0
	p.pendRestart = true
	p.mu.Unlock()
	if busy {
		p.ev.Add("info", "Santral", "Ayar değişikliği için Asterisk, süren aramalar bitince yeniden başlatılacak")
		return
	}
	p.Restart()
}

// Restart restarts Asterisk (active calls are cut).
func (p *PBX) Restart() {
	p.mu.Lock()
	p.pendRestart = false
	p.mu.Unlock()
	select {
	case p.restartCh <- struct{}{}:
	default:
	}
}

func (p *PBX) applyDebug(a *AMI, on bool) {
	if a == nil {
		return
	}
	if on {
		_, _ = a.Command("core set verbose 3")
		_, _ = a.Command("pjsip set logger on")
	} else {
		_, _ = a.Command("pjsip set logger off")
		_, _ = a.Command("core set verbose 0")
	}
}

func (p *PBX) SetSIPDebug(on bool) error {
	if err := p.store.SetSIPDebug(on); err != nil {
		return err
	}
	p.applyDebug(p.currentAMI(), on)
	p.RequestReload()
	return nil
}

// ---------------------------------------------------------------------------
// Maintenance: blocked IPs, external IP, deferred restarts, retention
// ---------------------------------------------------------------------------

func (p *PBX) maintenanceLoop(ctx context.Context) {
	tick := time.NewTicker(5 * time.Second)
	defer tick.Stop()
	lastPrune := time.Time{}
	lastNet := time.Now()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
		p.mu.Lock()
		expired := false
		for ip, until := range p.blocked {
			if time.Now().After(until) {
				delete(p.blocked, ip)
				expired = true
			}
		}
		pend := p.pendRestart && len(p.live) == 0 && p.state == "running"
		idle := len(p.live) == 0
		p.mu.Unlock()
		if expired {
			p.RequestReload()
		}
		if pend {
			p.Restart()
		}
		if time.Since(lastNet) > 10*time.Minute && idle {
			lastNet = time.Now()
			if p.refreshRuntime(ctx, false) {
				p.RequestReload()
			}
		}
		if time.Since(lastPrune) > 6*time.Hour {
			lastPrune = time.Now()
			p.pruneRecordings()
		}
	}
}

func (p *PBX) noteSecurityFailure(ip string) {
	if ip == "" || ip == "127.0.0.1" || !p.store.Settings().BlockFailed {
		return
	}
	p.mu.Lock()
	if _, ok := p.blocked[ip]; ok {
		p.mu.Unlock()
		return
	}
	f := p.fails[ip]
	if f == nil || time.Since(f.first) > 10*time.Minute {
		f = &failInfo{first: time.Now()}
		p.fails[ip] = f
	}
	f.count++
	block := f.count >= 5
	if block {
		p.blocked[ip] = time.Now().Add(time.Hour)
		delete(p.fails, ip)
	}
	p.mu.Unlock()
	if block {
		p.ev.Add("warn", "Güvenlik", fmt.Sprintf("%s 1 saat engellendi (5 hatalı SIP girişi)", ip))
		p.RequestReload()
	}
}

// BlockedIPs lists currently blocked addresses.
func (p *PBX) BlockedIPs() map[string]time.Time {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := map[string]time.Time{}
	for ip, t := range p.blocked {
		out[ip] = t
	}
	return out
}

func (p *PBX) Unblock(ip string) {
	p.mu.Lock()
	delete(p.blocked, ip)
	delete(p.fails, ip)
	p.mu.Unlock()
	p.RequestReload()
}

// ---------------------------------------------------------------------------
// AMI events
// ---------------------------------------------------------------------------

func (p *PBX) eventLoop(a *AMI) {
	for {
		select {
		case <-a.Done():
			return
		case m := <-a.Events:
			p.handleEvent(m)
		}
	}
}

func b64(s string) string {
	if s == "" {
		return ""
	}
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return s
	}
	return strings.TrimSpace(string(b))
}

func epochTime(s string) time.Time {
	n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil || n <= 0 {
		return time.Time{}
	}
	return time.Unix(n, 0)
}

func (p *PBX) handleEvent(m amiMsg) {
	switch m.Get("Event") {
	case "UserEvent":
		switch m.Get("UserEvent") {
		case "SantralStart":
			p.onCallStart(m)
		case "SantralState":
			p.onCallState(m)
		case "SantralEnd":
			p.onCallEnd(m)
		}
	case "Registry":
		p.onRegistry(m)
	case "ContactStatus", "FullyBooted", "PeerStatus":
		p.kickStatus()
	case "InvalidPassword", "ChallengeResponseFailed", "InvalidAccountID", "FailedACL":
		parts := strings.Split(m.Get("RemoteAddress"), "/")
		if len(parts) >= 3 {
			p.noteSecurityFailure(parts[2])
		}
	}
}

func (p *PBX) onCallStart(m amiMsg) {
	lc := &LiveCall{
		ID: m.Get("Call"), LineID: m.Get("Line"), Mode: m.Get("Mode"),
		Caller: b64(m.Get("CallerNum")), CallerName: b64(m.Get("CallerName")),
		State: "started", Start: time.Now(), Recording: m.Get("Rec") != "",
	}
	if lc.ID == "" {
		return
	}
	p.mu.Lock()
	p.live[lc.ID] = lc
	p.mu.Unlock()
}

func (p *PBX) onCallState(m amiMsg) {
	id := m.Get("Call")
	p.mu.Lock()
	defer p.mu.Unlock()
	lc := p.live[id]
	if lc == nil {
		return
	}
	switch st := m.Get("State"); st {
	case "answered":
		lc.State = st
		lc.By = m.Get("By")
		lc.AnsweredAt = time.Now()
	case "ringing":
		lc.State = st
		lc.Targets = m.Get("Targets")
	case "announcement", "noanswer", "voicemail", "busy":
		lc.State = st
	}
}

func (p *PBX) lineInfo(id string) (string, string) {
	if l, err := p.store.Line(id); err == nil {
		return l.Name, l.Number
	}
	return "Silinmiş hat", ""
}

func (p *PBX) extName(num string) string {
	if e, err := p.store.Extension(num); err == nil {
		return e.Name
	}
	return ""
}

func (p *PBX) baseRecord(id, lineID, mode, caller, callerName string, start, end time.Time) CallRecord {
	name, number := p.lineInfo(lineID)
	if start.IsZero() {
		start = end
	}
	if caller == "" {
		caller = "Gizli numara"
	}
	return CallRecord{
		ID: id, Start: start, End: end, LineID: lineID, LineName: name, LineNumber: number,
		Caller: caller, CallerName: callerName, Mode: mode, DurSec: int(end.Sub(start).Seconds()),
	}
}

func (p *PBX) onCallEnd(m amiMsg) {
	id := m.Get("Call")
	if id == "" {
		return
	}
	p.mu.Lock()
	lc := p.live[id]
	delete(p.live, id)
	p.mu.Unlock()

	start := epochTime(m.Get("Start"))
	end := epochTime(m.Get("End"))
	if end.IsZero() {
		end = time.Now()
	}
	caller, callerName := b64(m.Get("CallerNum")), b64(m.Get("CallerName"))
	if lc != nil {
		if start.IsZero() {
			start = lc.Start
		}
		if caller == "" {
			caller, callerName = lc.Caller, lc.CallerName
		}
	}
	mode := m.Get("Mode")
	rec := p.baseRecord(id, m.Get("Line"), mode, caller, callerName, start, end)
	ans := m.Get("Ans")
	vm := epochTime(m.Get("VM"))
	switch {
	case ans != "":
		rec.Result = "answered"
		rec.AnsweredBy = ans
		rec.AnsweredByName = p.extName(ans)
		if lc != nil && !lc.AnsweredAt.IsZero() {
			rec.TalkSec = int(end.Sub(lc.AnsweredAt).Seconds())
		}
	case !vm.IsZero() && end.Sub(vm) >= 3*time.Second:
		rec.Result = "voicemail"
		rec.TalkSec = int(end.Sub(vm).Seconds())
	case mode == "ann":
		rec.Result = "announcement"
	case mode == "busy":
		rec.Result = "busy"
	default:
		rec.Result = "missed"
	}
	p.ev.Add("info", rec.LineName, fmt.Sprintf("%s · %s · %s", rec.Caller, resultText(rec), fmtDur(rec.DurSec)))
	base := m.Get("Rec")
	if base != "" {
		select {
		case p.recJobs <- recJob{rec: rec, base: base, queued: time.Now()}:
			return
		default:
			p.log.Printf("kayıt kuyruğu dolu: %s", base)
		}
	}
	_ = p.calls.Add(rec)
}

func resultText(r CallRecord) string {
	switch r.Result {
	case "answered":
		if r.AnsweredByName != "" {
			return "cevaplandı (" + r.AnsweredBy + " " + r.AnsweredByName + ")"
		}
		return "cevaplandı (" + r.AnsweredBy + ")"
	case "voicemail":
		return "sesli mesaj bıraktı"
	case "announcement":
		return "anons dinledi"
	case "interrupted":
		return "yarıda kaldı"
	case "busy":
		return "meşgul verildi"
	default:
		return "cevapsız"
	}
}

// ---------------------------------------------------------------------------
// Line and phone status
// ---------------------------------------------------------------------------

func (p *PBX) kickStatus() {
	select {
	case p.statusCh <- struct{}{}:
	default:
	}
}

func (p *PBX) statusLoop(a *AMI) {
	t := time.NewTicker(20 * time.Second)
	defer t.Stop()
	for {
		p.refreshStatus(a)
		select {
		case <-a.Done():
			return
		case <-t.C:
		case <-p.statusCh:
			time.Sleep(700 * time.Millisecond)
		}
	}
}

func sipCauseText(cause string) string {
	c := strings.ToLower(cause)
	switch {
	case strings.HasPrefix(c, "401"), strings.HasPrefix(c, "403"), strings.Contains(c, "forbidden"), strings.Contains(c, "unauthorized"):
		return "Kullanıcı adı ya da şifre hatalı (" + cause + ")"
	case strings.HasPrefix(c, "404"):
		return "Sağlayıcı bu kullanıcıyı tanımıyor (" + cause + ")"
	case strings.HasPrefix(c, "408"), strings.Contains(c, "timeout"):
		return "Sağlayıcıya ulaşılamıyor (zaman aşımı)"
	case strings.HasPrefix(c, "503"), strings.HasPrefix(c, "500"):
		return "Sağlayıcı hizmet vermiyor (" + cause + ")"
	case cause == "":
		return ""
	default:
		return cause
	}
}

func (p *PBX) onRegistry(m amiMsg) {
	user := m.Get("Username")
	domain := m.Get("Domain")
	status := strings.ToLower(m.Get("Status"))
	cause := m.Get("Cause")
	for _, l := range p.store.Lines() {
		if !strings.Contains(user, l.Username+"@"+l.Server) && !(strings.Contains(domain, l.Server) && strings.Contains(user, l.Username)) {
			continue
		}
		p.mu.Lock()
		switch status {
		case "registered":
			delete(p.regCause, l.ID)
		case "rejected", "failed", "unregistered":
			if cause != "" {
				p.regCause[l.ID] = cause
			} else if status == "rejected" {
				p.regCause[l.ID] = "kayıt reddedildi"
			}
		}
		p.mu.Unlock()
	}
	p.kickStatus()
}

func (p *PBX) refreshStatus(a *AMI) {
	regs, err := a.ActionList("PJSIPShowRegistrationsOutbound", nil, 10*time.Second)
	if err != nil {
		p.log.Printf("kayıt durumu alınamadı: %v", err)
		return
	}
	regStatus := map[string]string{}
	for _, r := range regs {
		if r.Get("Event") == "OutboundRegistrationDetail" {
			regStatus[r.Get("ObjectName")] = r.Get("Status")
		}
	}
	contacts, err := a.ActionList("PJSIPShowContacts", nil, 10*time.Second)
	if err != nil {
		p.log.Printf("dahili durumu alınamadı: %v", err)
	}
	type cinfo struct {
		ua, addr, status string
		rtt              int
	}
	byAor := map[string]cinfo{}
	for _, c := range contacts {
		if c.Get("Event") != "ContactList" {
			continue
		}
		aor := c.Get("Aor")
		if aor == "" { // ObjectName is "<aor>;@<hash>"
			aor, _, _ = strings.Cut(c.Get("ObjectName"), ";@")
		}
		rtt, _ := strconv.Atoi(c.Get("RoundtripUsec"))
		addr := c.Get("ViaAddr")
		if addr == "" {
			addr = c.Get("Uri")
		}
		byAor[aor] = cinfo{ua: c.Get("UserAgent"), addr: addr, status: c.Get("Status"), rtt: rtt / 1000}
	}

	now := time.Now()
	lines := p.store.Lines()
	exts := p.store.Extensions()
	p.mu.Lock()
	var logs [][3]string
	for _, l := range lines {
		st, detail := "trying", ""
		raw, ok := regStatus[lineEndpoint(l.ID)]
		switch {
		case !l.Enabled:
			st = "disabled"
		case !ok:
			st = "trying"
		case strings.EqualFold(raw, "Registered"):
			st = "registered"
		case strings.EqualFold(raw, "Rejected"):
			st = "failed"
			detail = sipCauseText(p.regCause[l.ID])
			if detail == "" {
				detail = "Sağlayıcı kaydı reddetti"
			}
		default:
			if c := p.regCause[l.ID]; c != "" {
				st, detail = "failed", sipCauseText(c)
			}
		}
		old := p.lineSt[l.ID]
		if old == nil || old.State != st || old.Detail != detail {
			p.lineSt[l.ID] = &LineStatus{State: st, Detail: detail, Since: now}
			if old != nil || st == "failed" {
				switch st {
				case "registered":
					logs = append(logs, [3]string{"info", l.Name, "Hat kayıtlı"})
				case "failed":
					logs = append(logs, [3]string{"error", l.Name, "Kayıt başarısız: " + detail})
				}
			}
		}
	}
	for id := range p.lineSt {
		found := false
		for _, l := range lines {
			if l.ID == id {
				found = true
			}
		}
		if !found {
			delete(p.lineSt, id)
		}
	}
	for _, e := range exts {
		c, ok := byAor[e.Num]
		online := ok && !strings.EqualFold(c.status, "Unreachable") && !strings.EqualFold(c.status, "Removed")
		old := p.extSt[e.Num]
		ns := &ExtStatus{Online: online, Since: now}
		if old != nil {
			ns.LastSeen = old.LastSeen
			if old.Online == online {
				ns.Since = old.Since
			}
		}
		if online {
			ns.UA, ns.Addr, ns.RTTms, ns.LastSeen = c.ua, c.addr, c.rtt, now
		} else if old != nil && old.Online {
			ns.UA, ns.Addr = old.UA, old.Addr
		}
		if old != nil && old.Online != online {
			if online {
				logs = append(logs, [3]string{"info", "Dahili " + e.Num, "Bağlandı (" + c.ua + ")"})
			} else {
				logs = append(logs, [3]string{"warn", "Dahili " + e.Num, "Bağlantı kesildi"})
			}
		}
		p.extSt[e.Num] = ns
	}
	p.mu.Unlock()
	for _, l := range logs {
		p.ev.Add(l[0], l[1], l[2])
	}
}

// TestLine re-registers a line and waits briefly for the result.
func (p *PBX) TestLine(id string) LineStatus {
	a := p.currentAMI()
	if a == nil {
		return LineStatus{State: "unknown", Detail: "Santral çalışmıyor"}
	}
	name := lineEndpoint(id)
	_, _ = a.Action("PJSIPUnregister", map[string]string{"Registration": name}, 5*time.Second)
	time.Sleep(500 * time.Millisecond)
	p.mu.Lock()
	delete(p.regCause, id)
	p.mu.Unlock()
	_, _ = a.Action("PJSIPRegister", map[string]string{"Registration": name}, 5*time.Second)
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(time.Second)
		p.refreshStatus(a)
		p.mu.Lock()
		st := p.lineSt[id]
		p.mu.Unlock()
		if st != nil && (st.State == "registered" || st.State == "failed") {
			return *st
		}
	}
	return LineStatus{State: "trying", Detail: "Sağlayıcıdan henüz yanıt yok"}
}

// ---------------------------------------------------------------------------
// Snapshots for the web panel
// ---------------------------------------------------------------------------

type PBXState struct {
	State       string    `json:"state"`
	Message     string    `json:"message,omitempty"`
	Version     string    `json:"asteriskVersion,omitempty"`
	Since       time.Time `json:"since,omitempty"`
	ExtIP       string    `json:"extIp,omitempty"`
	LocalNets   []string  `json:"localNets,omitempty"`
	PendRestart bool      `json:"pendingRestart,omitempty"`
}

func (p *PBX) State() PBXState {
	p.mu.Lock()
	defer p.mu.Unlock()
	return PBXState{State: p.state, Message: p.stateMsg, Version: p.astVersion, Since: p.astStart,
		ExtIP: p.rt.ExtIP, LocalNets: append([]string(nil), p.rt.LocalNets...), PendRestart: p.pendRestart}
}

func (p *PBX) LineStatusOf(id string) LineStatus {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.state != "running" {
		return LineStatus{State: "unknown"}
	}
	if s := p.lineSt[id]; s != nil {
		return *s
	}
	return LineStatus{State: "trying"}
}

func (p *PBX) ExtStatusOf(num string) ExtStatus {
	p.mu.Lock()
	defer p.mu.Unlock()
	if s := p.extSt[num]; s != nil {
		c := *s
		if p.state != "running" {
			c.Online = false
		}
		return c
	}
	return ExtStatus{}
}

func (p *PBX) Live() []LiveCall {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]LiveCall, 0, len(p.live))
	for _, lc := range p.live {
		out = append(out, *lc)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Start.Before(out[j].Start) })
	return out
}

// AsteriskLog returns the tail of Asterisk's own log file.
func (p *PBX) AsteriskLog(maxBytes int64) string {
	path, err := p.rf.Path("/var/log/asterisk/messages.log")
	if err != nil {
		return ""
	}
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	st, _ := f.Stat()
	if st != nil && st.Size() > maxBytes {
		_, _ = f.Seek(-maxBytes, io.SeekEnd)
	}
	b, _ := io.ReadAll(f)
	s := string(b)
	if i := strings.IndexByte(s, '\n'); i >= 0 && st != nil && st.Size() > maxBytes {
		s = s[i+1:]
	}
	// keep the log small: rotate when it grows past 8 MB
	if st != nil && st.Size() > 8<<20 {
		if a := p.currentAMI(); a != nil {
			_, _ = a.Command("logger rotate")
		}
	}
	return s
}

// lanIP picks the address phones should use to reach the NAS.
func lanIP() string {
	addrs := lanAddresses()
	for _, a := range addrs {
		if strings.HasPrefix(a, "192.168.") {
			return a
		}
	}
	if len(addrs) > 0 {
		return addrs[0]
	}
	if conn, err := net.Dial("udp4", "192.0.2.1:9"); err == nil {
		defer conn.Close()
		return conn.LocalAddr().(*net.UDPAddr).IP.String()
	}
	return ""
}
