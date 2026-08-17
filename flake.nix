{
  description = "civitai — the Civitai command-line interface (App Blocks authoring & more)";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs =
    { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (
      system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
        lib = pkgs.lib;

        # Version/commit/date are stamped into main.{version,commit,date} via
        # ldflags so `civitai version` reports something meaningful for a Nix
        # build (mirroring the goreleaser stamps). `self.shortRev`/`self.rev`
        # are only present for a clean (committed) tree; fall back to the dirty
        # rev, then a static label for a rev-less build (e.g. a local dirty
        # `nix build`). `self.lastModifiedDate` is the flake's source date.
        version = self.shortRev or self.dirtyShortRev or "nix";
        commit = self.rev or self.dirtyRev or "unknown";
        date = self.lastModifiedDate or "unknown";

        civitai = pkgs.buildGoModule {
          pname = "civitai";
          inherit version;

          src = lib.cleanSource ./.;

          # Computed from go.sum (this module is not vendored). To recompute
          # after a go.mod/go.sum change: set this to `lib.fakeHash`, run
          # `nix build`, and copy the `got: sha256-…` value from the error.
          vendorHash = "sha256-l0RyxhS7+tdL9FngitiUhrciQbZfyDmTp5tP3Kd6NGQ=";

          # The module root is `package cli` (a library that embeds the schema);
          # the executable lives in ./cmd/civitai (package main). Build only it.
          subPackages = [ "cmd/civitai" ];

          # Pure-Go build → static binary, no C toolchain needed.
          env.CGO_ENABLED = 0;

          # Stamp version/commit/date into cmd/civitai/main.go, mirroring the
          # release ldflags in .goreleaser.yaml. `-s -w` strips debug info.
          ldflags = [
            "-s"
            "-w"
            "-X main.version=${version}"
            "-X main.commit=${commit}"
            "-X main.date=${date}"
          ];

          meta = {
            description = "Civitai CLI — author, validate, and ship App Blocks (and more)";
            homepage = "https://github.com/civitai/cli";
            license = lib.licenses.asl20;
            mainProgram = "civitai";
          };
        };
      in
      {
        packages.default = civitai;
        packages.civitai = civitai;

        apps.default = flake-utils.lib.mkApp {
          drv = civitai;
          name = "civitai";
        };

        devShells.default = pkgs.mkShell {
          packages = [
            pkgs.go
            pkgs.gopls
            pkgs.gotools
          ];
        };
      }
    );
}
