package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"
)

func newID(prefix string) string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return prefix + hex.EncodeToString(b)
}

func randomToken(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// randomPassword returns an easy-to-type password (no look-alike characters).
func randomPassword(n int) string {
	const alphabet = "abcdefghjkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	var sb strings.Builder
	for i := 0; i < n; i++ {
		v, _ := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
		sb.WriteByte(alphabet[v.Int64()])
	}
	return sb.String()
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	s := fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
	return strings.Replace(s, ".", ",", 1)
}

// writeFileAtomic writes via a temp file + rename so readers never see a
// half-written file.
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func copyFile(src, dst string, perm os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dst + ".part"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dst)
}

// moveFile renames, falling back to copy+delete across file systems.
func moveFile(src, dst string, perm os.FileMode) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	if err := copyFile(src, dst, perm); err != nil {
		return err
	}
	return os.Remove(src)
}

// linkOrCopy hard-links src to dst (same volume, no extra space) or copies it.
func linkOrCopy(src, dst string) error {
	_ = os.Remove(dst)
	if err := os.Link(src, dst); err == nil {
		return nil
	}
	return copyFile(src, dst, 0o644)
}

// slug turns a display name into a safe ASCII folder name ("Türkiye hattı" -> "Turkiye-hatti").
func slug(s string) string {
	repl := strings.NewReplacer("ç", "c", "Ç", "C", "ğ", "g", "Ğ", "G", "ı", "i", "İ", "I", "ö", "o", "Ö", "O",
		"ş", "s", "Ş", "S", "ü", "u", "Ü", "U", "ä", "a", "Ä", "A", "ß", "ss", "é", "e", "è", "e", "â", "a", "î", "i", "û", "u")
	s = repl.Replace(s)
	var sb strings.Builder
	dash := false
	for _, r := range s {
		switch {
		case r < 128 && (unicode.IsLetter(r) || unicode.IsDigit(r)):
			sb.WriteRune(r)
			dash = false
		case r == '.' || r == '_':
			sb.WriteRune(r)
			dash = false
		default:
			if !dash && sb.Len() > 0 {
				sb.WriteByte('-')
				dash = true
			}
		}
	}
	out := strings.Trim(sb.String(), "-.")
	if out == "" {
		out = "hat"
	}
	if len(out) > 48 {
		out = out[:48]
	}
	return out
}

// rotatingWriter is a small size-based log rotator (keeps one old file).
type rotatingWriter struct {
	mu   sync.Mutex
	path string
	max  int64
	f    *os.File
	size int64
}

func newRotatingWriter(path string, max int64) (*rotatingWriter, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	w := &rotatingWriter{path: path, max: max}
	if err := w.open(); err != nil {
		return nil, err
	}
	return w, nil
}

func (w *rotatingWriter) open() error {
	f, err := os.OpenFile(w.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return err
	}
	w.f, w.size = f, st.Size()
	return nil
}

func (w *rotatingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.size+int64(len(p)) > w.max {
		w.f.Close()
		_ = os.Rename(w.path, w.path+".1")
		if err := w.open(); err != nil {
			return 0, err
		}
	}
	n, err := w.f.Write(p)
	w.size += int64(n)
	return n, err
}

// fmtDur formats seconds as mm:ss or h:mm:ss.
func fmtDur(sec int) string {
	if sec < 0 {
		sec = 0
	}
	h, m, s := sec/3600, (sec/60)%60, sec%60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%02d:%02d", m, s)
}

// sinceText is a short Turkish "x ago" text.
func sinceText(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "az önce"
	case d < time.Hour:
		return fmt.Sprintf("%d dk önce", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%d saat önce", int(d.Hours()))
	default:
		return fmt.Sprintf("%d gün önce", int(d.Hours()/24))
	}
}
