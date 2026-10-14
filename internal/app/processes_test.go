package app

import (
	"reflect"
	"testing"
)

func TestExtractSandboxFlag(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		defaultVal   string
		wantSandbox  string
		wantCleanArg []string
	}{
		{
			name:         "no flags",
			args:         []string{"list"},
			defaultVal:   "default-sb",
			wantSandbox:  "default-sb",
			wantCleanArg: []string{"list"},
		},
		{
			name:         "empty args",
			args:         []string{},
			defaultVal:   "default-sb",
			wantSandbox:  "default-sb",
			wantCleanArg: []string{},
		},
		{
			name:         "long flag separate",
			args:         []string{"--sandbox", "custom-sb", "list"},
			defaultVal:   "default-sb",
			wantSandbox:  "custom-sb",
			wantCleanArg: []string{"list"},
		},
		{
			name:         "short flag separate",
			args:         []string{"-s", "custom-sb", "stop", "docker"},
			defaultVal:   "default-sb",
			wantSandbox:  "custom-sb",
			wantCleanArg: []string{"stop", "docker"},
		},
		{
			name:         "long flag equals",
			args:         []string{"--sandbox=custom-sb", "status", "docker"},
			defaultVal:   "default-sb",
			wantSandbox:  "custom-sb",
			wantCleanArg: []string{"status", "docker"},
		},
		{
			name:         "short flag equals",
			args:         []string{"-s=custom-sb", "logs"},
			defaultVal:   "default-sb",
			wantSandbox:  "custom-sb",
			wantCleanArg: []string{"logs"},
		},
		{
			name:         "terminator before flag",
			args:         []string{"--", "--sandbox", "foo"},
			defaultVal:   "default-sb",
			wantSandbox:  "default-sb",
			wantCleanArg: []string{"--sandbox", "foo"},
		},
		{
			name:         "flag before terminator",
			args:         []string{"--sandbox", "custom-sb", "--", "list"},
			defaultVal:   "default-sb",
			wantSandbox:  "custom-sb",
			wantCleanArg: []string{"list"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotSandbox, gotClean := extractSandboxFlag(tt.args, tt.defaultVal)
			if gotSandbox != tt.wantSandbox {
				t.Errorf(
					"extractSandboxFlag() gotSandbox = %v, want %v",
					gotSandbox,
					tt.wantSandbox,
				)
			}
			if !reflect.DeepEqual(gotClean, tt.wantCleanArg) {
				t.Errorf("extractSandboxFlag() gotClean = %v, want %v", gotClean, tt.wantCleanArg)
			}
		})
	}
}
