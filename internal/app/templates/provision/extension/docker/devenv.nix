{ pkgs, ... }:

{
  packages = [
    pkgs.docker
  ];

  # Mezha starts these once with devenv up -d and each session is a client.
  processes.mezha-docker = {
    start.enable = false;
    exec = ''
      rm -f /var/run/docker.pid
      exec dockerd --host=unix:///var/run/docker.sock --storage-driver=vfs
    '';
    ready.exec = "docker info >/dev/null";
    restart.on = "always";
    shutdown.grace = 30;
  };
}
