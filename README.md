# ollama-tray-go

A Linux system tray icon that shows which models an Ollama server has loaded, and unloads them on demand.

## Icon colors

* Green: at least one model is loaded.
* Grey: the server answers but nothing is loaded.
* Red: the server cannot be reached.

## Usage

* `--host` sets the Ollama server address. It falls back to `OLLAMA_HOST`, then `http://127.0.0.1:11434`. A host with no port uses 11434 for every scheme.
* `--interval` sets the time between status polls (default `5s`).
* `--version` prints the version, which is the short commit hash for a Nix build.

## Install on NixOS

Try it without installing:

```sh
nix run github:stephenbrannen/ollama-tray-go
```

Or add it to your system flake:

```nix
{
  inputs.ollama-tray.url = "github:stephenbrannen/ollama-tray-go";

  outputs = { nixpkgs, ollama-tray, ... }: {
    nixosConfigurations.myhost = nixpkgs.lib.nixosSystem {
      modules = [
        ({ pkgs, ... }: {
          environment.systemPackages = [ ollama-tray.packages.${pkgs.stdenv.hostPlatform.system}.default ];
        })
      ];
    };
  };
}
```

* While the repository is private, add `access-tokens = github.com=<token>` to `nix.conf` so Nix can fetch it.
* GNOME needs the AppIndicator extension to show tray icons.

## Autostart

The package installs `ollama-tray.desktop` under `share/applications`. To start the tray on login, copy or link it into `~/.config/autostart/`.

## Development

* `nix develop` provides go, gopls, golangci-lint, and pre-commit (x86_64-linux and aarch64-linux).
* `pre-commit install` enables the gofmt, go vet, and golangci-lint hooks.
* `go test -race -count=1 ./...` runs the tests.
* `GOOS=linux CGO_ENABLED=0 go build ./...` checks the Linux build.
* `nix build` builds the package. After any change to `go.mod` or `go.sum`, set `vendorHash` in `flake.nix` to `pkgs.lib.fakeHash`, run `nix build`, and paste the hash from the error.
