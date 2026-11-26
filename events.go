package main

import (
	"bufio"
	"encoding/json"
	"os"
	"sync"
	"time"
)

// Event is one line of the friendly activity log shown under Ayarlar.
type Event struct {
	Time   time.Time `json:"time"`
	Level  string    `json:"level"` // info | warn | error
	Source string    `json:"source,omitempty"`
	Text   string    `json:"text"`
}

type EventLog struct {
	mu   sync.Mutex
	buf  []Event
	max  int
	file *rotatingWriter
}

func OpenEventLog(path string) *EventLog {
	e := &EventLog{max: 600}
	if f, err := os.Open(path); err == nil {
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			var ev Event
			if json.Unmarshal(sc.Bytes(), &ev) == nil {
				e.buf = append(e.buf, ev)
			}
		}
		f.Close()
		if len(e.buf) > e.max {
			e.buf = e.buf[len(e.buf)-e.max:]
		}
	}
	e.file, _ = newRotatingWriter(path, 1<<20)
	return e
}

func (e *EventLog) Add(level, source, text string) {
	ev := Event{Time: time.Now(), Level: level, Source: source, Text: text}
	e.mu.Lock()
	e.buf = append(e.buf, ev)
	if len(e.buf) > e.max {
		e.buf = e.buf[len(e.buf)-e.max:]
	}
	e.mu.Unlock()
	if e.file != nil {
		b, _ := json.Marshal(ev)
		_, _ = e.file.Write(append(b, '\n'))
	}
}

// Recent returns the newest n events, newest first.
func (e *EventLog) Recent(n int) []Event {
	e.mu.Lock()
	defer e.mu.Unlock()
	if n > len(e.buf) {
		n = len(e.buf)
	}
	out := make([]Event, 0, n)
	for i := len(e.buf) - 1; i >= 0 && len(out) < n; i-- {
		out = append(out, e.buf[i])
	}
	return out
}
