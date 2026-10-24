//go:build !unix

package core

import "errors"

// Restart is not available on this system.
func Restart(exe string) error { return errors.New("restart is not supported here") }
