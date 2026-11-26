//go:build linux

package main

import (
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

const canRunPBX = true

// mountRuntime mounts /proc and a small /dev (tmpfs with the few device
// nodes Asterisk needs) inside the root file system.
func (r *Rootfs) mountRuntime() error {
	unmountAll(r.Dir, r.log)
	proc := filepath.Join(r.Dir, "proc")
	dev := filepath.Join(r.Dir, "dev")
	for _, d := range []string{proc, dev} {
		if st, err := os.Lstat(d); err == nil && st.Mode()&os.ModeSymlink != 0 {
			return &os.PathError{Op: "mount", Path: d, Err: syscall.ELOOP}
		}
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	if err := syscall.Mount("proc", proc, "proc", syscall.MS_NOSUID|syscall.MS_NODEV|syscall.MS_NOEXEC, ""); err != nil {
		return &os.PathError{Op: "mount proc", Path: proc, Err: err}
	}
	if err := syscall.Mount("tmpfs", dev, "tmpfs", syscall.MS_NOSUID|syscall.MS_NOEXEC, "mode=0755,size=256k"); err != nil {
		return &os.PathError{Op: "mount dev", Path: dev, Err: err}
	}
	nodes := []struct {
		name         string
		major, minor uint32
	}{{"null", 1, 3}, {"zero", 1, 5}, {"random", 1, 8}, {"urandom", 1, 9}, {"tty", 5, 0}}
	for _, n := range nodes {
		p := filepath.Join(dev, n.name)
		if err := syscall.Mknod(p, syscall.S_IFCHR|0o666, int(n.major<<8|n.minor)); err != nil {
			return &os.PathError{Op: "mknod", Path: p, Err: err}
		}
		_ = os.Chmod(p, 0o666)
	}
	_ = os.MkdirAll(filepath.Join(dev, "shm"), 0o1777)
	_ = os.Symlink("/proc/self/fd", filepath.Join(dev, "fd"))
	return nil
}

// unmountAll detaches every mount below dir (deepest first).
func unmountAll(dir string, lg *log.Logger) {
	for _, mp := range mountsUnder(dir) {
		if err := syscall.Unmount(mp, syscall.MNT_DETACH); err != nil && lg != nil {
			lg.Printf("ayrılamadı %s: %v", mp, err)
		}
	}
}

// chrootCmd prepares a command that runs inside the root file system.
func (r *Rootfs) chrootCmd(name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	cmd.Dir = "/"
	cmd.Env = []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "HOME=/var/lib/asterisk", "LANG=C.UTF-8", "TERM=dumb"}
	cmd.SysProcAttr = &syscall.SysProcAttr{Chroot: r.Dir, Setpgid: true}
	return cmd
}

func killGroup(cmd *exec.Cmd, sig syscall.Signal) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = syscall.Kill(-cmd.Process.Pid, sig)
}
