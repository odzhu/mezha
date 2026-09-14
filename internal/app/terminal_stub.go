//go:build !unix

package app

func terminalSize(_ int) (uint32, uint32) {
	return 80, 24
}

func terminalIsTerminal(_ int) bool {
	return false
}

func makeRawTerminal(_ int) (func(), error) {
	return nil, nil
}
