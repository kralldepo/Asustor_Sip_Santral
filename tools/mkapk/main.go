// mkapk builds an ASUSTOR App Central package (APKG 2.0) from a staging
// directory, mirroring ASUSTOR's apkg-tools_py3.py "create" command:
//
//	<pkg>_<version>_<arch>.apk = zip(stored) {
//	    apkg-version     "2.0\n"
//	    control.tar.gz   contents of CONTROL/ as "./..."
//	    data.tar.gz      everything except CONTROL as "./..."
//	}
//
// Unlike the Python tool it does not need root: all entries are written as
// root:root with explicit modes (CONTROL files 0644, *.sh/*.py 0755).
//
// Usage: go run ./tools/mkapk <stage-dir> <out-dir>
// (copied from the Remote Runner project)
package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "kullanım: mkapk <stage-dir> <out-dir>")
		os.Exit(2)
	}
	out, err := build(os.Args[1], os.Args[2])
	if err != nil {
		fmt.Fprintln(os.Stderr, "HATA:", err)
		os.Exit(1)
	}
	fmt.Println(out)
}

type general struct {
	Package      string `json:"package"`
	Version      string `json:"version"`
	Architecture string `json:"architecture"`
	Firmware     string `json:"firmware"`
}

func build(stage, outDir string) (string, error) {
	control := filepath.Join(stage, "CONTROL")
	raw, err := os.ReadFile(filepath.Join(control, "config.json"))
	if err != nil {
		return "", err
	}
	var cfg struct {
		General general `json:"general"`
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return "", fmt.Errorf("config.json: %w", err)
	}
	g := cfg.General
	for k, v := range map[string]string{"package": g.Package, "version": g.Version, "architecture": g.Architecture, "firmware": g.Firmware} {
		if strings.TrimSpace(v) == "" || strings.Contains(v, "__") {
			return "", fmt.Errorf("config.json general.%s boş/geçersiz: %q", k, v)
		}
	}
	if !regexp.MustCompile(`^[a-zA-Z0-9.+-]+$`).MatchString(g.Package) {
		return "", fmt.Errorf("paket adı geçersiz: %q", g.Package)
	}
	if _, err := os.Stat(filepath.Join(control, "icon.png")); err != nil {
		return "", fmt.Errorf("CONTROL/icon.png yok")
	}
	now := time.Now()
	if s := os.Getenv("SOURCE_DATE_EPOCH"); s != "" {
		var sec int64
		fmt.Sscan(s, &sec)
		now = time.Unix(sec, 0)
	}

	controlTgz, err := tarGz(control, now, nil)
	if err != nil {
		return "", fmt.Errorf("control.tar.gz: %w", err)
	}
	dataTgz, err := tarGz(stage, now, func(rel string) bool {
		// Same rule as apkg-tools: anything with "CONTROL" in its path is excluded.
		return strings.Contains(rel, "CONTROL")
	})
	if err != nil {
		return "", fmt.Errorf("data.tar.gz: %w", err)
	}

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return "", err
	}
	name := filepath.Join(outDir, g.Package+"_"+g.Version+"_"+g.Architecture+".apk")
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range []struct {
		name string
		data []byte
	}{{"apkg-version", []byte("2.0\n")}, {"control.tar.gz", controlTgz}, {"data.tar.gz", dataTgz}} {
		h := &zip.FileHeader{Name: e.name, Method: zip.Store, Modified: now}
		h.SetMode(0o644)
		w, err := zw.CreateHeader(h)
		if err != nil {
			return "", err
		}
		if _, err := w.Write(e.data); err != nil {
			return "", err
		}
	}
	if err := zw.Close(); err != nil {
		return "", err
	}
	if err := os.WriteFile(name, buf.Bytes(), 0o644); err != nil {
		return "", err
	}
	return name, nil
}

// tarGz archives root as "./..." entries (like `tar.add('.')` in Python).
func tarGz(root string, mtime time.Time, skip func(rel string) bool) ([]byte, error) {
	var paths []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)
		if rel != "." && skip != nil && skip(rel) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() == ".DS_Store" || strings.HasPrefix(d.Name(), "._") {
			return nil
		}
		paths = append(paths, rel)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	var buf bytes.Buffer
	gz, _ := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	tw := tar.NewWriter(gz)
	for _, rel := range paths {
		full := filepath.Join(root, filepath.FromSlash(rel))
		st, err := os.Lstat(full)
		if err != nil {
			return nil, err
		}
		name := "./" + rel
		if rel == "." {
			name = "./"
		}
		h := &tar.Header{Name: name, ModTime: mtime, Uid: 0, Gid: 0, Uname: "root", Gname: "root", Format: tar.FormatUSTAR}
		switch {
		case st.IsDir():
			h.Typeflag = tar.TypeDir
			if !strings.HasSuffix(h.Name, "/") {
				h.Name += "/"
			}
			h.Mode = 0o755
		case st.Mode().IsRegular():
			h.Typeflag = tar.TypeReg
			h.Size = st.Size()
			h.Mode = 0o644
			if st.Mode()&0o111 != 0 || strings.HasSuffix(rel, ".sh") || strings.HasSuffix(rel, ".py") {
				h.Mode = 0o755
			}
		default:
			return nil, fmt.Errorf("desteklenmeyen dosya türü: %s", rel)
		}
		if len(h.Name) > 100 {
			h.Format = tar.FormatPAX
		}
		if err := tw.WriteHeader(h); err != nil {
			return nil, err
		}
		if h.Typeflag == tar.TypeReg {
			b, err := os.ReadFile(full)
			if err != nil {
				return nil, err
			}
			if strings.HasSuffix(rel, ".sh") {
				b = bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n")) // dos2unix
			}
			if _, err := tw.Write(b); err != nil {
				return nil, err
			}
		}
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	if err := gz.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
