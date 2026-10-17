# ollama-tray-go

A Linux system tray icon that shows which models an Ollama server has loaded, and unloads them on demand.

## Icon colors

* Green: at least one model is loaded.
* Grey: the server answers but nothing is loaded.
* Red: the server cannot be reached.

## Usage

* `--host` sets the Ollama server address. It falls back to `OLLAMA_HOST`, then `http://127.0.0.1:11434`. A host with no port uses 11434 for every scheme. Only `http` and `https` are accepted, and user info, a query or a fragment in the address is an error.
* `--interval` sets the time between status polls (default `5s`, minimum `1s`).
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

## Install from a release

Each tagged release has a `tar.gz` archive per architecture (`linux_amd64`, `linux_arm64`) and a `checksums.txt`.

* Download the archive for your architecture from the Releases page, or with `gh release download <tag> -R stephenbrannen/ollama-tray-go`.
* Verify it with `sha256sum -c checksums.txt --ignore-missing`.
* Verify its build provenance with `gh attestation verify <archive> -R stephenbrannen/ollama-tray-go`. This proves the archive came from this repository's release workflow. Releases get an attestation only while the repository is public.
* Extract it with `tar xzf <archive>`, then move `ollama-tray` to `~/.local/bin`.
* Copy `ollama-tray.desktop` to `~/.config/autostart/` to start the tray on login.
* While the repository is private, only collaborators can download releases.

### Withdraw a bad release

* Delete the release and its tag with `gh release delete <tag> --cleanup-tag -R stephenbrannen/ollama-tray-go`.
* Fix the problem on `main`, then tag a patch version (for example `v0.1.2`). Do not reuse the deleted tag.

The release workflow runs `go vet` and the tests, and refuses a tag whose commit is not on `main`.

## Autostart

The package installs `ollama-tray.desktop` under `share/applications`. To start the tray on login, copy or link it into `~/.config/autostart/`.

## Development

* `nix develop` provides go and gopls (x86_64-linux and aarch64-linux). golangci-lint and govulncheck are `tool` directives in `go.mod`, so run them as `go tool golangci-lint` and `go tool govulncheck`.
* `git config core.hooksPath .githooks` enables the hooks. `pre-commit` runs gofmt, `go mod tidy -diff`, `go vet`, golangci-lint, and govulncheck. `pre-push` runs the tests.
* `go run -tags task ./tasks/coverage.go` runs `go test -race -count=1 ./...` and fails below 80% statement coverage. The count leaves out `cmd/ollama-tray` and `internal/tray`, which are startup and tray wiring. `go test -tags task ./tasks/` tests that program.
* `GOOS=linux CGO_ENABLED=0 go build ./...` checks the Linux build.
* `nix fmt` formats `flake.nix`. `nix build` builds the package and `nix flake check` also runs every package's tests.

### Update `vendorHash`

After any change to `go.mod` or `go.sum`, including Dependabot `gomod` pull requests, `nix build` fails with a hash mismatch until `vendorHash` in `flake.nix` matches.

* Set `vendorHash` to `pkgs.lib.fakeHash` and run `nix build`.
* Copy the hash after `got:` in the error into `flake.nix`.
* Commit it to the same branch. For a Dependabot pull request, push to its branch (`gh pr checkout <number>`).
* The `nix` CI job prints a reminder when it fails.
