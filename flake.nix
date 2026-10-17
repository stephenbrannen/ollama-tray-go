{
  description = "ollama-tray-go: Linux system tray for Ollama";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";

  outputs = { self, nixpkgs }:
    let
      systems = [ "x86_64-linux" "aarch64-linux" ];
      forAllSystems = nixpkgs.lib.genAttrs systems;
    in
    {
      packages = forAllSystems (system:
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
            vendorHash = "sha256-HejxjovHSSbUDFuEvjJkYN5Tep0doYjtGS+U9Zj6tho=";
            env.CGO_ENABLED = 0;
            ldflags = [
              "-s"
              "-w"
              "-X github.com/stephenbrannen/ollama-tray-go/internal/version.Version=${version}"
            ];
            postInstall = ''
              install -Dm644 ${./ollama-tray.desktop} $out/share/applications/ollama-tray.desktop
            '';
          };
        });

      apps = forAllSystems (system: {
        default = {
          type = "app";
          program = "${self.packages.${system}.default}/bin/ollama-tray";
        };
      });

      devShells = forAllSystems (system:
        let pkgs = nixpkgs.legacyPackages.${system}; in
        {
          default = pkgs.mkShell {
            packages = with pkgs; [ go gopls golangci-lint pre-commit ];
          };
        });
    };
}
