{ pkgs, ... }:

{
  packages = [
    pkgs.less
    pkgs.unixtools.col
    pkgs.git
    pkgs.lazygit
    pkgs.gh
    pkgs.go
    pkgs.groff
    pkgs.procps
  ];

  # Render manpages safely when command output is captured instead of attached
  # to a terminal. grotty then emits overstrikes and col removes them.
  env.GROFF_NO_SGR = "1";
  env.MANPAGER = "col -b";
}
