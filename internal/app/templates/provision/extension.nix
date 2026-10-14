{ lib, ... }:

let
  extensionsUserDir = ./extensions-user;
  userImports =
    if builtins.pathExists extensionsUserDir then
      let
        entries = builtins.readDir extensionsUserDir;
        importEntry = name: type:
          if type == "directory" then
            if builtins.pathExists (extensionsUserDir + "/${name}/devenv.nix") then
              [ (extensionsUserDir + "/${name}/devenv.nix") ]
            else if builtins.pathExists (extensionsUserDir + "/${name}/default.nix") then
              [ (extensionsUserDir + "/${name}/default.nix") ]
            else
              [ ]
          else if (type == "regular" || type == "symlink") && lib.hasSuffix ".nix" name then
            [ (extensionsUserDir + "/${name}") ]
          else
            [ ];
      in
        lib.concatLists (lib.mapAttrsToList importEntry entries)
    else
      [ ];
in
{
  imports = [
    ./extension/docker/devenv.nix
    ./extension/k3s/devenv.nix
  ] ++ userImports;
}
