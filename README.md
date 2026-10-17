# ollama-tray-go

A Linux system tray icon that shows which models an Ollama server has loaded, and unloads them on demand.

## Icon colors

* Green: at least one model is loaded.
* Grey: the server answers but nothing is loaded, or the first check has not returned yet.
* Red: the server cannot be reached, answers with an error, or sends a reply the tray cannot read.

Hover over the icon to see the server address, each loaded model with its VRAM use and the time until the server unloads it, and the last failed action. A failed action leaves the tooltip after 60 seconds.

## Keeping models loaded

The server unloads an idle model after its keep-alive time, 5 minutes by default. To keep models loaded longer, set `OLLAMA_KEEP_ALIVE` on the server. On NixOS:

```nix
services.ollama.environmentVariables.OLLAMA_KEEP_ALIVE = "24h";
```

The tooltip shows when each model unloads on its own, for example `llama3:8b (4.7 GB VRAM, unloads in 3h 12m)`. The time rounds down to whole minutes. A model kept loaded forever (keep-alive `-1`) shows `kept loaded`. The tooltip shows no time when the server sends none or sends one the tray cannot read.

## Menu

* `Stop <model>` unloads that model. One item shows per loaded model, up to 8. The menu has no room for more, so use `Stop all` for the rest.
* `Keep loaded` is a submenu with one entry per loaded model, up to 8. An entry keeps that model in memory until you stop it, whatever the server's keep-alive setting is. A model whose tooltip shows `kept loaded` has no entry, and the submenu hides when no model can be kept.
* `Stop all` unloads every loaded model, including any past the eighth. It shows only while two or more models are loaded.
* `Refresh now` polls the server at once.
* `Quit` closes the tray.

Requests run in the background, so the menu stays responsive while a large model unloads.

The tray polls every `--interval` and runs until you pick `Quit` or it receives SIGINT or SIGTERM.

## Requirements

* A Linux desktop with a system tray. GNOME needs the AppIndicator extension to show tray icons.

## Usage

* `--host` sets the Ollama server address. It falls back to `OLLAMA_HOST`, then `http://127.0.0.1:11434`.
  * No scheme means `http`. The scheme is case-insensitive, and only `http` and `https` are accepted.
  * No port means 11434 for every scheme, including `https`.
  * No host (for example `:8080`) means `127.0.0.1`, not `localhost`, because `localhost` can resolve to IPv6 while Ollama listens on IPv4.
  * A path is kept, without its trailing slash, so a reverse proxy under a prefix works.
  * User info, a query, a fragment, a scheme other than `http` or `https`, and a port outside 1 to 65535 are errors.
* `--interval` sets the time between status polls. It takes Go duration syntax, such as `10s`, `1m` or `1m30s`. The default is `5s` and the minimum is `1s`.
* `--version` prints `ollama-tray` and the version, and exits. The version depends on how the binary was built:
  * Nix build: the short commit hash, for example `ollama-tray 7329d7d`. A tree with uncommitted changes gets a `-dirty` suffix, or just `dirty` when Nix has no commit to name.
  * Release archive: the release number without a `v`, for example `ollama-tray 0.1.1`.
  * Plain `go build`: `ollama-tray dev`.
* `--help` prints the usage to stderr and exits with 0.

An invalid flag, host or interval prints an error to stderr and exits with 2.

## Packages

* `cmd/ollama-tray` parses the flags and starts the tray.
* `internal/ollama` is the client for `/api/ps` and `/api/generate`, plus host parsing.
* `internal/status` maps a poll result to a tray state.
* `internal/menu` decides the menu items and tooltip text.
* `internal/icon` draws the icons at runtime.
* `internal/tray` connects those pieces to the system tray and runs the poll loop.
* `internal/version` holds the build version.
* `tasks` holds the coverage gate program, and `tasks/e2e` runs the end-to-end test.
* `e2e` holds the end-to-end test and its Docker setup.

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

## Install from a release

Each tagged release has a `tar.gz` archive per architecture (`linux_amd64`, `linux_arm64`) and a `checksums.txt`.

* Download the archive for your architecture and `checksums.txt` from the Releases page, or with `gh release download <tag> -R stephenbrannen/ollama-tray-go -p '*_linux_amd64.tar.gz' -p checksums.txt` (use `arm64` on ARM).
* Verify it with `sha256sum -c checksums.txt --ignore-missing`.
* Verify its build provenance with `gh attestation verify <archive> -R stephenbrannen/ollama-tray-go`. This proves the archive came from this repository's release workflow. Releases get an attestation only while the repository is public.
* Extract it with `tar xzf <archive>`, then move `ollama-tray` to `~/.local/bin`.
* Copy `ollama-tray.desktop` to `~/.config/autostart/` to start the tray on login. It runs `ollama-tray` from your `PATH`.
* While the repository is private, only collaborators can download releases.

### Withdraw a bad release

* Delete the release and its tag with `gh release delete <tag> --cleanup-tag -R stephenbrannen/ollama-tray-go`.
* Fix the problem on `main`, then tag a patch version (for example `v0.1.2`). Do not reuse the deleted tag.

The release workflow runs `go vet` and the tests, and refuses a tag whose commit is not on `main`.

## Autostart

The package installs `ollama-tray.desktop` under `share/applications`. To start the tray on login, copy or link it into `~/.config/autostart/`.

## Development

* `nix develop` provides go and gopls (x86_64-linux and aarch64-linux). golangci-lint and govulncheck are `tool` directives in `go.mod`, so run them as `go tool golangci-lint` and `go tool govulncheck`.
* `git config core.hooksPath .githooks` enables the hooks. `pre-commit` runs gofmt, `go mod tidy -diff`, `go vet`, golangci-lint, and govulncheck. `pre-push` runs the coverage gate and the `tasks` tests.
* `go run -tags task ./tasks/coverage.go` runs `go test -race -count=1 ./...` and fails below 80% statement coverage. The count leaves out `cmd/ollama-tray` and `internal/tray`, which are startup and tray wiring. `go test -tags task ./tasks/` tests that program.
* `GOOS=linux CGO_ENABLED=0 go build ./...` checks the Linux build.
* `nix fmt` formats `flake.nix`. `nix build` builds the package and `nix flake check` also runs every package's tests.

### End-to-end test

```sh
go run -tags task ./tasks/e2e
```

This needs Docker with Compose. It starts a real Ollama server and a test container with a session D-Bus and a stub tray host (a StatusNotifierWatcher). The test runs the tray binary and reads its icon, tooltip and menu over D-Bus the way a desktop panel does. It loads a model, clicks the model in the `Keep loaded` submenu and checks that the tooltip shows `kept loaded`, then clicks `Stop <model>` and checks that Ollama unloads the model within 0.5 seconds. Then it closes the TCP proxy between the tray and Ollama to check the red unreachable state. The first run downloads `smollm2:135m` into a Docker volume, and later runs reuse it. Arguments after the command go to the test binary, for example `-test.run`.

The test is behind the `e2e` build tag, so `go test ./...` and the coverage gate do not run it. CI runs it in the `e2e` workflow.

### Update `vendorHash`

After any change to `go.mod` or `go.sum`, including Dependabot `gomod` pull requests, `nix build` fails with a hash mismatch until `vendorHash` in `flake.nix` matches.

* Set `vendorHash` to `pkgs.lib.fakeHash` and run `nix build`.
* Copy the hash after `got:` in the error into `flake.nix`.
* Commit it to the same branch. For a Dependabot pull request, push to its branch (`gh pr checkout <number>`).
* The `nix` CI job prints a reminder when it fails.
