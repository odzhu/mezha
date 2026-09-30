{ pkgs, lib, config, ... }:

{
  packages = [
    pkgs.k3s
    pkgs.kubectl
  ];

  env.KUBECONFIG = "/var/lib/rancher/k3s/k3s.yaml";

  # Ensure Docker is enabled if k3s is enabled, since k3s runs with --docker.
  processes.docker.start.enable = lib.mkIf config.processes.k3s.start.enable true;

  # Mezha starts these once with devenv up -d and each session is a client.
  processes.k3s = {
    start.enable = false;
    after = lib.mkIf config.processes.k3s.start.enable [ "devenv:processes:docker" ];
    exec = ''
      exec k3s server \
        --data-dir /var/lib/rancher/k3s \
        --node-name mezha-k3s \
        --https-listen-port=16443 \
        --docker \
        --write-kubeconfig /var/lib/rancher/k3s/k3s.yaml \
        --write-kubeconfig-mode 644
    '';
    ready.exec = ''
      kubectl --kubeconfig /var/lib/rancher/k3s/k3s.yaml get nodes --no-headers |
        awk '$2 ~ /^Ready/ { ready=1 } END { exit !ready }'
    '';
    restart.on = "always";
    shutdown.grace = 30;
  };
}
