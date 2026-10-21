{
  pkgs ? import <nixpkgs> { },
  version ? "dev",
  commit ? "unknown",
  # Accepts a bool or the string "false" (from `nix-build --argstr`), so image
  # builds that already ran tests elsewhere can skip the checkPhase.
  doCheck ? true,
}:
let
  inherit (pkgs.lib) cleanSource cleanSourceWith;
in
pkgs.buildGoModule {
  pname = "birthday";
  inherit version;

  src = cleanSourceWith {
    filter =
      name: _:
      !(
        (baseNameOf name) == "Dockerfile"
        || (baseNameOf name) == "README.md"
        || (baseNameOf name) == "AGENTS.md"
        || (baseNameOf name) == "nix"
      );
    src = cleanSource ../.;
  };
  ldflags = [
    "-s"
    "-w"
    "-X github.com/Banh-Canh/birthday/cmd.Version=${version}"
    "-X github.com/Banh-Canh/birthday/cmd.Commit=${commit}"
  ];

  env.CGO_ENABLED = 0;

  # Standard library only: nothing to vendor. Set a hash here once go.mod
  # gains a dependency.
  vendorHash = null;

  doCheck = doCheck == true || doCheck == "true";

  meta = with pkgs.lib; {
    description = "A little birthday countdown with an I.O.U.";
    homepage = "https://github.com/Banh-Canh/birthday";
    license = licenses.asl20;
    platforms = platforms.linux;
    mainProgram = "birthday";
  };
}
