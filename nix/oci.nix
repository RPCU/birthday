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
  # Claude Code (unfree) runs the chat friend.
  unfree = import pkgs.path {
    inherit (pkgs) system;
    config.allowUnfreePredicate = p: pkgs.lib.getName p == "claude-code";
  };
in
pkgs.dockerTools.streamLayeredImage {
  name = imageName;
  tag = dockerVersion;
  contents = [
    binaries
    unfree.claude-code
    # A shell to sign Claude Code in: `claude auth login`.
    pkgs.bashInteractive
    pkgs.coreutils
    pkgs.cacert
    (pkgs.dockerTools.fakeNss.override {
      extraPasswdLines = [ "birthday:x:1001:0:Birthday:/home/birthday:/bin/bash" ];
    })
  ];
  fakeRootCommands = ''
    ln -s bin/birthday birthday
    mkdir -p home/birthday tmp
    chown 1001:0 home/birthday
    chmod 0770 home/birthday
    chmod 1777 tmp
  '';
  config = {
    User = "1001:0";
    Entrypoint = [ "/birthday" ];
    # OpenChoreo workloads cannot set command/args, so the image's default
    # command must start the server.
    Cmd = [ "serve" ];
    WorkingDir = "/home/birthday";
    Env = [
      "HOME=/home/birthday"
      # The chat friend's own Claude login, apart from any other settings.
      "CLAUDE_CONFIG_DIR=/home/birthday/.claude"
      "DISABLE_AUTOUPDATER=1"
      "PATH=/bin"
      "SSL_CERT_FILE=/etc/ssl/certs/ca-bundle.crt"
    ];
    ExposedPorts."8080/tcp" = { };
  };
}
