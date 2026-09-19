{ pkgs, lib, config, inputs, ... }:

{
  # https://devenv.sh/basics/
  env.GREET = "devenv";
  env.CGO_ENABLED = "1";


  # https://devenv.sh/packages/
  packages = [ pkgs.git pkgs.gitleaks ];

  # https://devenv.sh/languages/
  languages.go.enable = true;

  containers."mezha" = {
    name = "mezha";

    # Optional styling or multi-layer configuration
    # See more configuration attributes at https://devenv.sh
  };

  # https://devenv.sh/processes/
  # processes.dev.exec = "${lib.getExe pkgs.watchexec} -n -- ls -la";

  # https://devenv.sh/services/
  # services.postgres.enable = true;

  # https://devenv.sh/scripts/
  # scripts.hello.exec = ''
  #   echo hello from $GREET
  # '';

  # https://devenv.sh/basics/
  # enterShell = ''
  #   hello         # Run scripts directly
  #   git --version # Use packages
  # '';

  # https://devenv.sh/tasks/
  # tasks = {
  #   "myproj:setup".exec = "mytool build";
  #   "devenv:enterShell".after = [ "myproj:setup" ];
  # };

  # https://devenv.sh/tests/
  # enterTest = ''
  #   echo "Running tests"
  #   git --version | grep --color=auto "${pkgs.git.version}"
  # '';

  # https://devenv.sh/git-hooks/
  # git-hooks.hooks.shellcheck.enable = true;
  git-hooks.hooks.golangci-lint.enable = true;
  git-hooks.hooks.golines.enable = true;
  git-hooks.hooks.check-added-large-files.enable = true;
  git-hooks.hooks.check-case-conflicts.enable = true;
  git-hooks.hooks.check-merge-conflicts.enable = true;
  git-hooks.hooks.check-executables-have-shebangs.enable = true;
  git-hooks.hooks.check-shebang-scripts-are-executable.enable = true;
  git-hooks.hooks.check-symlinks.enable = true;
  git-hooks.hooks.end-of-file-fixer.enable = true;
  git-hooks.hooks.trim-trailing-whitespace.enable = true;
  git-hooks.hooks.check-json.enable = true;
  git-hooks.hooks.check-toml.enable = true;
  git-hooks.hooks.check-yaml.enable = true;
  git-hooks.hooks.check-vcs-permalinks.enable = true;
  git-hooks.hooks.actionlint.enable = true;

  # See full reference at https://devenv.sh/reference/options/

  git-hooks.hooks.gitleaks = {
    enable = true;
    name = "gitleaks";
    entry = "${pkgs.gitleaks}/bin/gitleaks protect --staged --verbose";
    language = "system";
    pass_filenames = false;
  };
}
