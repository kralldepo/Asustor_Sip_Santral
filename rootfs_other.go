//go:build !linux

package main

import (
	"errors"
	"log"
	"os/exec"
	"syscall"
)

// On macOS (development) Asterisk cannot run; the panel works without it.
const canRunPBX = false

var errNoLinux = errors.New("Asterisk yalnız Linux (NAS) üzerinde çalışır")

func (r *Rootfs) mountRuntime() error { return errNoLinux }

func unmountAll(dir string, lg *log.Logger) {}

func (r *Rootfs) chrootCmd(name string, args ...string) *exec.Cmd {
	return exec.Command("false")
}

func killGroup(cmd *exec.Cmd, sig syscall.Signal) {
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Signal(sig)
	}
}
