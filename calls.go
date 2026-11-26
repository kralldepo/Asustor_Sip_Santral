package main

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// CallRecord is one finished incoming call.
type CallRecord struct {
	ID             string    `json:"id"`
	Start          time.Time `json:"start"`
	End            time.Time `json:"end"`
	LineID         string    `json:"lineId"`
	LineName       string    `json:"lineName"`
	LineNumber     string    `json:"lineNumber,omitempty"`
	Caller         string    `json:"caller"`
	CallerName     string    `json:"callerName,omitempty"`
	Mode           string    `json:"mode"`
	Result         string    `json:"result"` // answered | missed | voicemail | announcement | busy | interrupted
	AnsweredBy     string    `json:"answeredBy,omitempty"`
	AnsweredByName string    `json:"answeredByName,omitempty"`
	DurSec         int       `json:"durSec"`
	TalkSec        int       `json:"talkSec,omitempty"`
	Rec            string    `json:"rec,omitempty"` // relative to the recordings folder
	RecBytes       int64     `json:"recBytes,omitempty"`
	RecSec         int       `json:"recSec,omitempty"`
	RecRoot        string    `json:"recRoot,omitempty"` // recordings folder at the time of the call
}

// CallLog keeps all call records in memory, persisted as monthly JSON-lines
// files (calls/2026-11.jsonl).
type CallLog struct {
	mu    sync.RWMutex
	dir   string
	items []CallRecord // newest first
}

func OpenCallLog(dir string) (*CallLog, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	c := &CallLog{dir: dir}
	files, _ := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	seen := map[string]bool{}
	for _, f := range files {
		fh, err := os.Open(f)
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(fh)
		sc.Buffer(make([]byte, 64<<10), 1<<20)
		for sc.Scan() {
			var r CallRecord
			if json.Unmarshal(sc.Bytes(), &r) == nil && r.ID != "" {
				if seen[r.ID] { // later lines update earlier ones
					for i := range c.items {
						if c.items[i].ID == r.ID {
							c.items[i] = r
						}
					}
					continue
				}
				seen[r.ID] = true
				c.items = append(c.items, r)
			}
		}
		fh.Close()
	}
	sort.SliceStable(c.items, func(i, j int) bool { return c.items[i].Start.After(c.items[j].Start) })
	return c, nil
}

func monthFile(dir string, t time.Time) string {
	return filepath.Join(dir, t.UTC().Format("2006-01")+".jsonl")
}

func (c *CallLog) Add(r CallRecord) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := range c.items {
		if c.items[i].ID == r.ID {
			c.items[i] = r
			return c.appendLocked(r)
		}
	}
	i := sort.Search(len(c.items), func(i int) bool { return !c.items[i].Start.After(r.Start) })
	c.items = append(c.items, CallRecord{})
	copy(c.items[i+1:], c.items[i:])
	c.items[i] = r
	return c.appendLocked(r)
}

func (c *CallLog) appendLocked(r CallRecord) error {
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(monthFile(c.dir, r.Start), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(b, '\n'))
	return err
}

func (c *CallLog) Get(id string) (CallRecord, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, r := range c.items {
		if r.ID == id {
			return r, true
		}
	}
	return CallRecord{}, false
}

type CallQuery struct {
	From, To time.Time
	LineID   string
	Result   string
	Search   string
	Limit    int
	Offset   int
}

type CallSummary struct {
	Total        int   `json:"total"`
	Answered     int   `json:"answered"`
	Missed       int   `json:"missed"`
	Voicemail    int   `json:"voicemail"`
	Announcement int   `json:"announcement"`
	Busy         int   `json:"busy"`
	RecSec       int   `json:"recSec"`
	RecBytes     int64 `json:"recBytes"`
	Files        int   `json:"files"`
}

func (s *CallSummary) add(r CallRecord) {
	s.Total++
	switch r.Result {
	case "answered":
		s.Answered++
	case "voicemail":
		s.Voicemail++
	case "announcement":
		s.Announcement++
	case "busy":
		s.Busy++
	default:
		s.Missed++
	}
	if r.Rec != "" {
		s.Files++
		s.RecSec += r.RecSec
		s.RecBytes += r.RecBytes
	}
}

func digitsOnly(s string) string {
	var sb strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

func (q CallQuery) match(r CallRecord) bool {
	if !q.From.IsZero() && r.Start.Before(q.From) {
		return false
	}
	if !q.To.IsZero() && !r.Start.Before(q.To) {
		return false
	}
	if q.LineID != "" && r.LineID != q.LineID {
		return false
	}
	if q.Result != "" {
		res := r.Result
		if res == "interrupted" {
			res = "missed"
		}
		if res != q.Result {
			return false
		}
	}
	if q.Search != "" {
		s := strings.ToLower(q.Search)
		d := digitsOnly(q.Search)
		hit := strings.Contains(strings.ToLower(r.CallerName), s) || strings.Contains(strings.ToLower(r.Caller), s)
		if !hit && d != "" && strings.Contains(digitsOnly(r.Caller), d) {
			hit = true
		}
		if !hit {
			return false
		}
	}
	return true
}

// Query returns the matching page, the number of matches and their summary.
func (c *CallLog) Query(q CallQuery) ([]CallRecord, int, CallSummary) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	var sum CallSummary
	out := []CallRecord{}
	n := 0
	for _, r := range c.items {
		if !q.match(r) {
			continue
		}
		sum.add(r)
		if n >= q.Offset && (q.Limit <= 0 || len(out) < q.Limit) {
			out = append(out, r)
		}
		n++
	}
	return out, n, sum
}

// Recent returns the newest n records.
func (c *CallLog) Recent(n int) []CallRecord {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if n > len(c.items) {
		n = len(c.items)
	}
	return append([]CallRecord(nil), c.items[:n]...)
}

// CountByLine counts calls per line since t.
func (c *CallLog) CountByLine(since time.Time) map[string]int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	m := map[string]int{}
	for _, r := range c.items {
		if r.Start.Before(since) {
			break
		}
		m[r.LineID]++
	}
	return m
}

// CountByExt counts answered calls per extension since t.
func (c *CallLog) CountByExt(since time.Time) map[string]int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	m := map[string]int{}
	for _, r := range c.items {
		if r.Start.Before(since) {
			break
		}
		if r.AnsweredBy != "" {
			m[r.AnsweredBy]++
		}
	}
	return m
}

// Prune drops records that started before cutoff and calls del for each
// recording file. The month files are rewritten.
func (c *CallLog) Prune(cutoff time.Time, del func(CallRecord)) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	var keep []CallRecord
	removed := 0
	for _, r := range c.items {
		if r.Start.Before(cutoff) {
			if r.Rec != "" && del != nil {
				del(r)
			}
			removed++
			continue
		}
		keep = append(keep, r)
	}
	if removed == 0 {
		return 0
	}
	c.items = keep
	c.rewriteLocked()
	return removed
}

func (c *CallLog) rewriteLocked() {
	byMonth := map[string][]CallRecord{}
	for _, r := range c.items {
		f := monthFile(c.dir, r.Start)
		byMonth[f] = append(byMonth[f], r)
	}
	files, _ := filepath.Glob(filepath.Join(c.dir, "*.jsonl"))
	for _, f := range files {
		if _, ok := byMonth[f]; !ok {
			_ = os.Remove(f)
		}
	}
	for f, recs := range byMonth {
		sort.Slice(recs, func(i, j int) bool { return recs[i].Start.Before(recs[j].Start) })
		var sb strings.Builder
		for _, r := range recs {
			b, _ := json.Marshal(r)
			sb.Write(b)
			sb.WriteByte('\n')
		}
		_ = writeFileAtomic(f, []byte(sb.String()), 0o600)
	}
}

// Compact rewrites the files once at start so updates do not pile up.
func (c *CallLog) Compact() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.rewriteLocked()
}
