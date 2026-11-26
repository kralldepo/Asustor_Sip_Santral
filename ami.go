package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

// amiMsg is one AMI packet (response or event).
type amiMsg struct {
	h      map[string]string // lower-case keys, last value wins
	output []string          // "Output:" lines of Command responses
}

func (m amiMsg) Get(k string) string { return m.h[strings.ToLower(k)] }

type amiList struct {
	events []amiMsg
	done   chan struct{}
}

// AMI is a small Asterisk Manager Interface client.
type AMI struct {
	addr, user, secret string
	log                *log.Logger

	mu      sync.Mutex
	conn    net.Conn
	w       *bufio.Writer
	pending map[string]chan amiMsg
	lists   map[string]*amiList
	nextID  uint64
	closed  chan struct{}

	Events chan amiMsg
}

var errAMIClosed = errors.New("AMI bağlantısı kapalı")

func NewAMI(addr, user, secret string, lg *log.Logger) *AMI {
	return &AMI{addr: addr, user: user, secret: secret, log: lg, Events: make(chan amiMsg, 2048)}
}

// Connect dials and logs in. It returns once the login succeeded.
func (a *AMI) Connect(ctx context.Context) error {
	d := net.Dialer{Timeout: 3 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", a.addr)
	if err != nil {
		return err
	}
	r := bufio.NewReaderSize(conn, 64<<10)
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	banner, err := r.ReadString('\n')
	if err != nil || !strings.HasPrefix(banner, "Asterisk Call Manager") {
		conn.Close()
		return fmt.Errorf("AMI karşılama alınamadı: %v", err)
	}
	_ = conn.SetReadDeadline(time.Time{})
	a.mu.Lock()
	a.conn = conn
	a.w = bufio.NewWriter(conn)
	a.pending = map[string]chan amiMsg{}
	a.lists = map[string]*amiList{}
	a.closed = make(chan struct{})
	closed := a.closed
	a.mu.Unlock()
	go a.readLoop(r, conn, closed)

	resp, err := a.Action("Login", map[string]string{"Username": a.user, "Secret": a.secret, "Events": "on"}, 5*time.Second)
	if err != nil {
		a.Close()
		return err
	}
	if !strings.EqualFold(resp.Get("Response"), "Success") {
		a.Close()
		return fmt.Errorf("AMI girişi reddedildi: %s", resp.Get("Message"))
	}
	go a.keepAlive(closed)
	return nil
}

func (a *AMI) Close() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.conn != nil {
		a.conn.Close()
		a.conn = nil
	}
}

// Done is closed when the connection ends.
func (a *AMI) Done() <-chan struct{} {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed == nil {
		c := make(chan struct{})
		close(c)
		return c
	}
	return a.closed
}

func (a *AMI) Connected() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.conn != nil
}

func (a *AMI) keepAlive(closed chan struct{}) {
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-closed:
			return
		case <-t.C:
			if _, err := a.Action("Ping", nil, 10*time.Second); err != nil {
				a.log.Printf("AMI ping başarısız: %v", err)
				a.Close()
				return
			}
		}
	}
}

func (a *AMI) readLoop(r *bufio.Reader, conn net.Conn, closed chan struct{}) {
	defer func() {
		conn.Close()
		a.mu.Lock()
		if a.conn == conn {
			a.conn = nil
		}
		for id, ch := range a.pending {
			close(ch)
			delete(a.pending, id)
		}
		for id, l := range a.lists {
			close(l.done)
			delete(a.lists, id)
		}
		a.mu.Unlock()
		close(closed)
	}()
	msg := amiMsg{h: map[string]string{}}
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			if len(msg.h) > 0 {
				a.dispatch(msg)
			}
			msg = amiMsg{h: map[string]string{}}
			continue
		}
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		k = strings.ToLower(strings.TrimSpace(k))
		v = strings.TrimSpace(v)
		if k == "output" {
			msg.output = append(msg.output, v)
			continue
		}
		msg.h[k] = v
	}
}

func (a *AMI) dispatch(m amiMsg) {
	id := m.Get("ActionID")
	if m.Get("Response") != "" {
		a.mu.Lock()
		ch := a.pending[id]
		delete(a.pending, id)
		a.mu.Unlock()
		if ch != nil {
			ch <- m
		}
		return
	}
	if id != "" {
		a.mu.Lock()
		l := a.lists[id]
		if l != nil {
			if strings.EqualFold(m.Get("EventList"), "Complete") {
				delete(a.lists, id)
				close(l.done)
			} else {
				l.events = append(l.events, m)
			}
		}
		a.mu.Unlock()
		if l != nil {
			return
		}
	}
	select {
	case a.Events <- m:
	default:
		a.log.Printf("AMI olay kuyruğu dolu, olay atlandı: %s", m.Get("Event"))
	}
}

func (a *AMI) send(action string, fields map[string]string, list *amiList) (string, chan amiMsg, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.conn == nil {
		return "", nil, errAMIClosed
	}
	a.nextID++
	id := "s" + strconv.FormatUint(a.nextID, 10)
	ch := make(chan amiMsg, 1)
	a.pending[id] = ch
	if list != nil {
		a.lists[id] = list
	}
	var sb strings.Builder
	sb.WriteString("Action: " + action + "\r\nActionID: " + id + "\r\n")
	for k, v := range fields {
		v = strings.NewReplacer("\r", " ", "\n", " ").Replace(v)
		sb.WriteString(k + ": " + v + "\r\n")
	}
	sb.WriteString("\r\n")
	_ = a.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if _, err := a.w.WriteString(sb.String()); err != nil {
		delete(a.pending, id)
		delete(a.lists, id)
		return "", nil, err
	}
	if err := a.w.Flush(); err != nil {
		delete(a.pending, id)
		delete(a.lists, id)
		return "", nil, err
	}
	return id, ch, nil
}

// Action sends an action and waits for its response.
func (a *AMI) Action(action string, fields map[string]string, timeout time.Duration) (amiMsg, error) {
	id, ch, err := a.send(action, fields, nil)
	if err != nil {
		return amiMsg{}, err
	}
	select {
	case m, ok := <-ch:
		if !ok {
			return amiMsg{}, errAMIClosed
		}
		return m, nil
	case <-time.After(timeout):
		a.mu.Lock()
		delete(a.pending, id)
		a.mu.Unlock()
		return amiMsg{}, fmt.Errorf("AMI %s zaman aşımı", action)
	}
}

// ActionList sends an action whose result is a list of events.
func (a *AMI) ActionList(action string, fields map[string]string, timeout time.Duration) ([]amiMsg, error) {
	// The list collector is registered together with the action so that no
	// event of the list can arrive before it.
	l := &amiList{done: make(chan struct{})}
	id, ch, err := a.send(action, fields, l)
	if err != nil {
		return nil, err
	}
	deadline := time.After(timeout)
	select {
	case m, ok := <-ch:
		if !ok {
			return nil, errAMIClosed
		}
		if strings.EqualFold(m.Get("Response"), "Error") {
			a.mu.Lock()
			delete(a.lists, id)
			a.mu.Unlock()
			msg := m.Get("Message")
			if strings.Contains(strings.ToLower(msg), "no ") || strings.Contains(strings.ToLower(msg), "not found") {
				return nil, nil // empty list
			}
			return nil, fmt.Errorf("AMI %s: %s", action, msg)
		}
	case <-deadline:
		return nil, fmt.Errorf("AMI %s zaman aşımı", action)
	}
	select {
	case <-l.done:
		a.mu.Lock()
		ev := l.events
		a.mu.Unlock()
		return ev, nil
	case <-deadline:
		a.mu.Lock()
		delete(a.lists, id)
		a.mu.Unlock()
		return nil, fmt.Errorf("AMI %s listesi zaman aşımı", action)
	}
}

// Command runs an Asterisk CLI command and returns its output.
func (a *AMI) Command(cmd string) (string, error) {
	m, err := a.Action("Command", map[string]string{"Command": cmd}, 15*time.Second)
	if err != nil {
		return "", err
	}
	if strings.EqualFold(m.Get("Response"), "Error") {
		return "", fmt.Errorf("%s: %s %s", cmd, m.Get("Message"), strings.Join(m.output, " / "))
	}
	return strings.Join(m.output, "\n"), nil
}
