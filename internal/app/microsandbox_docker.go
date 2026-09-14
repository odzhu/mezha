//go:build cgo

package app

import "strings"

const dockerPath = "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"

// dockerCommand runs a command alongside a Docker daemon in the same
// Microsandbox exec job. Exec jobs have isolated runtime namespaces, so a
// daemon started by one job is not reachable from another one.
func dockerCommand(command string, args []string) (string, []string) {
	commandLine := shellQuote(command)
	for _, arg := range args {
		commandLine += " " + shellQuote(arg)
	}

	script := `set -eu
PATH=` + dockerPath + `
export PATH
if ! docker --context default info >/dev/null 2>&1; then
  rm -f /var/run/docker.pid
  dockerd --host=unix:///var/run/docker.sock --storage-driver=vfs >/tmp/mezha-dockerd.log 2>&1 &
  for _ in $(seq 1 30); do
    if docker --context default info >/dev/null 2>&1; then
      break
    fi
    sleep 1
  done
  if ! docker --context default info; then
    cat /tmp/mezha-dockerd.log >&2 || true
    exit 1
  fi
fi
docker context use default >/dev/null
exec ` + commandLine

	return "sh", []string{"-c", strings.TrimSpace(script)}
}
