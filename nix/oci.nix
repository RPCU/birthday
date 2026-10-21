{
  pkgs ? import <nixpkgs> { },
  dockerVersion ? "0.0.0",
  commit ? "unknown",
  doCheck ? true,
  imageName ? "zot.rpcu.io/public/birthday",
}:
let
  binaries = pkgs.callPackage ./binaries.nix {
    version = dockerVersion;
    inherit commit doCheck;
  };
in
pkgs.dockerTools.streamLayeredImage {
  name = imageName;
  tag = dockerVersion;
  contents = [
    binaries
    (pkgs.dockerTools.fakeNss.override {
      extraPasswdLines = [ "birthday:x:1001:0:Birthday:/var/empty:/noshell" ];
    })
  ];
  fakeRootCommands = ''
    ln -s bin/birthday birthday
  '';
  config = {
    User = "1001:0";
    Entrypoint = [ "/birthday" ];
    # OpenChoreo workloads cannot set command/args, so the image's default
    # command must start the server.
    Cmd = [ "serve" ];
    ExposedPorts."8080/tcp" = { };
  };
}
