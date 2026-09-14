//go:build unix

package app

import (
	"golang.org/x/sys/unix"
)

func terminalSize(fd int) (uint32, uint32) {
	ws, err := unix.IoctlGetWinsize(fd, unix.TIOCGWINSZ)
	if err != nil || ws == nil || ws.Col == 0 || ws.Row == 0 {
		return 80, 24
	}
	return uint32(ws.Col), uint32(ws.Row)
}

func terminalIsTerminal(fd int) bool {
	_, err := unix.IoctlGetTermios(fd, termiosReadReq)
	return err == nil
}

func makeRawTerminal(fd int) (func(), error) {
	termios, err := unix.IoctlGetTermios(fd, termiosReadReq)
	if err != nil {
		return nil, nil
	}

	original := *termios
	raw := original
	raw.Iflag &^= unix.IGNBRK | unix.BRKINT | unix.PARMRK | unix.ISTRIP | unix.INLCR | unix.IGNCR | unix.ICRNL | unix.IXON
	raw.Oflag &^= unix.OPOST
	raw.Lflag &^= unix.ECHO | unix.ECHONL | unix.ICANON | unix.ISIG | unix.IEXTEN
	raw.Cflag &^= unix.CSIZE | unix.PARENB
	raw.Cflag |= unix.CS8
	raw.Cc[unix.VMIN] = 1
	raw.Cc[unix.VTIME] = 0

	if err := unix.IoctlSetTermios(fd, termiosWriteReq, &raw); err != nil {
		return nil, err
	}

	return func() {
		_ = unix.IoctlSetTermios(fd, termiosWriteReq, &original)
	}, nil
}
