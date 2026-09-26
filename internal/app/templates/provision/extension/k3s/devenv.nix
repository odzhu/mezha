{ pkgs, ... }:

{
  packages = [
    pkgs.k3s
    pkgs.kubectl
  ];

  env.KUBECONFIG = "/var/lib/rancher/k3s/k3s.yaml";

  # Mezha starts these once with devenv up -d and each session is a client.
  processes.mezha-k3s = {
    start.enable = false;
    after = [ "devenv:processes:mezha-docker" ];
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
