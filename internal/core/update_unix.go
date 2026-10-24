//go:build unix

package core

import (
	"os"
	"syscall"
)

// Restart replaces this process with exe, same arguments and environment.
func Restart(exe string) error {
	return syscall.Exec(exe, os.Args, os.Environ())
}
