{
  description = "ollama-tray-go: Linux system tray for Ollama";

  # Nix asks once whether to trust these settings, then downloads the
  # prebuilt package instead of compiling it.
  nixConfig = {
    extra-substituters = [ "https://ollama-tray.cachix.org" ];
    extra-trusted-public-keys = [
      "ollama-tray.cachix.org-1:VsA+mvlRdugWzqzTzrRhqRpJ25Sc9tbL6z4or9oHc+U="
    ];
  };

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";

  outputs =
    { self, nixpkgs }:
    let
      systems = [
        "x86_64-linux"
        "aarch64-linux"
      ];
      forAllSystems = nixpkgs.lib.genAttrs systems;
    in
    {
      packages = forAllSystems (
        system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
          version = self.shortRev or self.dirtyShortRev or "dirty";
        in
        {
          default = pkgs.buildGoModule {
            pname = "ollama-tray";
            inherit version;
            src = self;
            subPackages = [ "cmd/ollama-tray" ];
            # Must be updated whenever go.mod or go.sum change.
            vendorHash = "sha256-P9bTFYeB/z4LhYfOkLszjiXeSridHn4b9DOs5U8lR4g=";
            env.CGO_ENABLED = 0;
            ldflags = [
              "-s"
              "-w"
              "-X github.com/stephenbrannen/ollama-tray-go/internal/version.Version=${version}"
            ];
            postInstall = ''
              install -Dm644 ${./ollama-tray.desktop} $out/share/applications/ollama-tray.desktop
            '';
            meta = {
              description = "Linux system tray for Ollama";
              license = pkgs.lib.licenses.mit;
              mainProgram = "ollama-tray";
              platforms = pkgs.lib.platforms.linux;
            };
          };
        }
      );

      checks = forAllSystems (system: {
        # The package build only tests cmd/ollama-tray. This runs every
        # package's tests under the race detector, which needs cgo.
        tests = self.packages.${system}.default.overrideAttrs (old: {
          pname = "ollama-tray-tests";
          subPackages = [ "./..." ];
          # The version test expects an unstamped binary.
          ldflags = [ ];
          checkFlags = [ "-race" ];
          env = old.env // {
            CGO_ENABLED = 1;
          };
        });
      });

      formatter = forAllSystems (system: nixpkgs.legacyPackages.${system}.nixfmt);

      apps = forAllSystems (system: {
        default = {
          type = "app";
          program = "${self.packages.${system}.default}/bin/ollama-tray";
        };
      });

      devShells = forAllSystems (
        system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
        in
        {
          default = pkgs.mkShell {
            packages = with pkgs; [
              go
              gopls
            ];
          };
        }
      );
    };
}
