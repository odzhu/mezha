//go:build darwin || freebsd || openbsd || netbsd || dragonfly

package app

import "golang.org/x/sys/unix"

const (
	termiosReadReq  = unix.TIOCGETA
	termiosWriteReq = unix.TIOCSETA
)
