//go:build unix

package main

import (
	"os/exec"
	"syscall"
)

// detach puts cmd in its own session, so closing the terminal does not stop it.
func detach(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} }
