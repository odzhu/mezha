package app

import (
	"os"
	"testing"
)

func TestTerminalFunctions(t *testing.T) {
	// With an invalid file descriptor like -1
	cols, rows := terminalSize(-1)
	if cols != 80 || rows != 24 {
		t.Errorf("terminalSize(-1) = (%d, %d), want (80, 24)", cols, rows)
	}

	isTerm := terminalIsTerminal(-1)
	if isTerm {
		t.Errorf("terminalIsTerminal(-1) = true, want false")
	}

	restore, err := makeRawTerminal(-1)
	if err != nil {
		t.Errorf("makeRawTerminal(-1) err = %v, want nil", err)
	}
	if restore != nil {
		t.Errorf("expected restore func to be nil for non-terminal fd")
	}
}

func TestMakeRawTerminalWithPtmx(t *testing.T) {
	ptmx, err := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		t.Skipf("cannot open /dev/ptmx: %v", err)
	}
	defer func() { _ = ptmx.Close() }()

	cols, rows := terminalSize(int(ptmx.Fd()))
	if cols == 0 || rows == 0 {
		t.Errorf("expected positive terminal size, got (%d, %d)", cols, rows)
	}

	restore, err := makeRawTerminal(int(ptmx.Fd()))
	if err != nil {
		t.Fatalf("makeRawTerminal error: %v", err)
	}
	if restore != nil {
		restore()
	}
}
