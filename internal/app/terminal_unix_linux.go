//go:build linux

package app

import "golang.org/x/sys/unix"

const (
	termiosReadReq  = unix.TCGETS
	termiosWriteReq = unix.TCSETS
)
