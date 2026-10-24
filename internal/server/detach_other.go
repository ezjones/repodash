//go:build !unix

package server

import "os/exec"

func detach(cmd *exec.Cmd) {}
