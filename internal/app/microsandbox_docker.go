//go:build cgo

package app

const managedDevenvPath = "/sandbox"

// dockerCommand enters Mezha's managed devenv environment. Its enterShell
// tasks provision Docker and k3s before the requested command is started.
func dockerCommand(command string, args []string, _ bool) (string, []string) {
	return "devenv", append(
		[]string{"shell", "--from", "path:" + managedDevenvPath, "--", command},
		args...,
	)
}
