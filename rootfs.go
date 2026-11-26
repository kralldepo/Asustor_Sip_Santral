package main

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Rootfs is the Alpine root file system Asterisk runs in (chroot). It is
// unpacked from the bundled rootfs.tar.gz and re-created after upgrades;
// nothing in it needs to survive (sounds and voices are linked in from the
// data folder on every start).
type Rootfs struct {
	Dir     string
	Tarball string
	log     *log.Logger
	UID     int // "asterisk" user inside the root file system
	GID     int
}

const rootfsMarker = ".santral-tarball"

func (r *Rootfs) tarballStamp() (string, error) {
	st, err := os.Stat(r.Tarball)
	if err != nil {
		return "", fmt.Errorf("rootfs paketi bulunamadı (%s): %w", r.Tarball, err)
	}
	return fmt.Sprintf("%d-%d", st.Size(), st.ModTime().Unix()), nil
}

// Ensure unpacks the root file system when it is missing or outdated.
// progress receives short Turkish status texts.
func (r *Rootfs) Ensure(progress func(string)) error {
	stamp, err := r.tarballStamp()
	if err != nil {
		return err
	}
	if b, err := os.ReadFile(filepath.Join(r.Dir, rootfsMarker)); err == nil && strings.TrimSpace(string(b)) == stamp {
		return r.loadIDs()
	}
	progress("Asterisk ortamı açılıyor (ilk kurulumda 1–2 dakika sürebilir)")
	r.log.Printf("rootfs açılıyor: %s -> %s", r.Tarball, r.Dir)
	tmp := r.Dir + ".new"
	if err := removeTreeNoMounts(tmp); err != nil {
		return err
	}
	if err := os.MkdirAll(tmp, 0o755); err != nil {
		return err
	}
	if err := extractTarGz(r.Tarball, tmp); err != nil {
		_ = removeTreeNoMounts(tmp)
		return fmt.Errorf("rootfs açılamadı: %w", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, rootfsMarker), []byte(stamp+"\n"), 0o644); err != nil {
		return err
	}
	unmountAll(r.Dir, r.log)
	old := r.Dir + ".old"
	_ = removeTreeNoMounts(old)
	if _, err := os.Stat(r.Dir); err == nil {
		if err := os.Rename(r.Dir, old); err != nil {
			return err
		}
	}
	if err := os.Rename(tmp, r.Dir); err != nil {
		return err
	}
	_ = removeTreeNoMounts(old)
	r.log.Printf("rootfs hazır")
	return r.loadIDs()
}

// loadIDs reads the asterisk user/group ids from the root file system.
func (r *Rootfs) loadIDs() error {
	r.UID, r.GID = 0, 0
	f, err := os.Open(filepath.Join(r.Dir, "etc", "passwd"))
	if err != nil {
		return err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		p := strings.Split(sc.Text(), ":")
		if len(p) >= 4 && p[0] == "asterisk" {
			r.UID, _ = strconv.Atoi(p[2])
			r.GID, _ = strconv.Atoi(p[3])
			return nil
		}
	}
	return errors.New("rootfs içinde asterisk kullanıcısı yok")
}

// Path returns the host path of a path inside the root file system and
// refuses to go through symbolic links (an absolute link would otherwise
// point into the NAS's own file system).
func (r *Rootfs) Path(inside string) (string, error) {
	inside = filepath.Clean("/" + inside)
	cur := r.Dir
	parts := strings.Split(strings.TrimPrefix(inside, "/"), "/")
	for i, p := range parts {
		if p == "" {
			continue
		}
		cur = filepath.Join(cur, p)
		st, err := os.Lstat(cur)
		if err != nil {
			if os.IsNotExist(err) {
				continue // will be created
			}
			return "", err
		}
		if st.Mode()&os.ModeSymlink != 0 && i < len(parts) {
			return "", fmt.Errorf("rootfs yolu bağlantı içeriyor: %s", inside)
		}
	}
	return cur, nil
}

// MkdirOwned creates directories inside the root file system owned by the
// asterisk user.
func (r *Rootfs) MkdirOwned(inside string, mode os.FileMode) (string, error) {
	p, err := r.Path(inside)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(p, mode); err != nil {
		return "", err
	}
	_ = os.Chmod(p, mode)
	_ = os.Lchown(p, r.UID, r.GID)
	return p, nil
}

// WriteFile writes a file inside the root file system.
func (r *Rootfs) WriteFile(inside string, data []byte, mode os.FileMode, owned bool) error {
	p, err := r.Path(inside)
	if err != nil {
		return err
	}
	if err := writeFileAtomic(p, data, mode); err != nil {
		return err
	}
	if owned {
		_ = os.Lchown(p, r.UID, r.GID)
	}
	return nil
}

// chownTree hands a directory tree to the asterisk user (no symlink following).
func (r *Rootfs) chownTree(inside string) {
	p, err := r.Path(inside)
	if err != nil {
		return
	}
	_ = filepath.Walk(p, func(path string, info os.FileInfo, err error) error {
		if err == nil {
			_ = os.Lchown(path, r.UID, r.GID)
		}
		return nil
	})
}

// extractTarGz unpacks src into dst. Entries that would leave dst or pass
// through a symbolic link are rejected.
func extractTarGz(src, dst string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(bufio.NewReaderSize(f, 1<<20))
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	type dirMode struct {
		path string
		mode os.FileMode
	}
	var dirs []dirMode
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		name := filepath.Clean(strings.TrimPrefix(h.Name, "./"))
		if name == "." || name == "" {
			continue
		}
		if filepath.IsAbs(name) || name == ".." || strings.HasPrefix(name, "../") {
			return fmt.Errorf("geçersiz yol: %s", h.Name)
		}
		target := filepath.Join(dst, name)
		if err := checkNoSymlinkParents(dst, name); err != nil {
			return err
		}
		mode := os.FileMode(h.Mode & 0o7777)
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			dirs = append(dirs, dirMode{target, mode})
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			_ = os.Remove(target)
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			if err := out.Close(); err != nil {
				return err
			}
			if err := os.Chmod(target, mode); err != nil {
				return err
			}
			_ = os.Lchown(target, h.Uid, h.Gid)
		case tar.TypeSymlink:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			_ = os.Remove(target)
			if err := os.Symlink(h.Linkname, target); err != nil {
				return err
			}
			_ = os.Lchown(target, h.Uid, h.Gid)
		case tar.TypeLink:
			ln := filepath.Clean(strings.TrimPrefix(h.Linkname, "./"))
			if filepath.IsAbs(ln) || strings.HasPrefix(ln, "../") {
				return fmt.Errorf("geçersiz bağlantı: %s", h.Linkname)
			}
			if err := checkNoSymlinkParents(dst, ln); err != nil {
				return err
			}
			_ = os.Remove(target)
			if err := os.Link(filepath.Join(dst, ln), target); err != nil {
				return err
			}
		default:
			// device nodes, fifos: not needed (/dev is a tmpfs at run time)
		}
	}
	for i := len(dirs) - 1; i >= 0; i-- {
		_ = os.Chmod(dirs[i].path, dirs[i].mode)
	}
	return nil
}

func checkNoSymlinkParents(root, rel string) error {
	cur := root
	parts := strings.Split(rel, string(filepath.Separator))
	for _, p := range parts[:len(parts)-1] {
		cur = filepath.Join(cur, p)
		st, err := os.Lstat(cur)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if st.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("arşiv girdisi bağlantı üzerinden yazmaya çalışıyor: %s", rel)
		}
	}
	return nil
}

// removeTreeNoMounts deletes a tree, first detaching anything still mounted
// inside it so that the removal can never reach into another file system.
func removeTreeNoMounts(dir string) error {
	if _, err := os.Lstat(dir); os.IsNotExist(err) {
		return nil
	}
	unmountAll(dir, nil)
	if len(mountsUnder(dir)) > 0 {
		return fmt.Errorf("%s altında hâlâ bağlı dosya sistemi var, silinmedi", dir)
	}
	return os.RemoveAll(dir)
}

// mountsUnder lists mount points at or below dir (longest first).
func mountsUnder(dir string) []string {
	b, err := os.ReadFile("/proc/self/mounts")
	if err != nil {
		return nil
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil
	}
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		abs = real
	}
	var out []string
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		mp := strings.ReplaceAll(f[1], `\040`, " ")
		if mp == abs || strings.HasPrefix(mp, abs+"/") {
			out = append(out, mp)
		}
	}
	// deepest first
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if len(out[j]) > len(out[i]) {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}
