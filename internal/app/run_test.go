package app

import "testing"

func TestInteractiveTTYEnabledExplicitValue(t *testing.T) {
	interactive := true
	if !interactiveTTYEnabled(&interactive) {
		t.Error("interactiveTTYEnabled(true) = false, want true")
	}

	interactive = false
	if interactiveTTYEnabled(&interactive) {
		t.Error("interactiveTTYEnabled(false) = true, want false")
	}
}
