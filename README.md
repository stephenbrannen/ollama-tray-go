# ollama-tray-go

A system tray icon for Linux and macOS that shows which models an Ollama server has loaded, and unloads them on demand.

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

* Linux: a desktop with a system tray. GNOME needs the AppIndicator extension to show tray icons.
* macOS: the icon shows in the menu bar. See [macOS](#macos).

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

The build needs a nixpkgs with Go 1.26 or newer. `go.mod` requires Go 1.26, and nixpkgs sets `GOTOOLCHAIN=local`, so Go never downloads a newer toolchain. `nixos-26.05` works; an older nixpkgs fails the build.

Try it without installing:

```sh
nix run github:stephenbrannen/ollama-tray-go
```

Nix downloads the prebuilt package from the `ollama-tray` Cachix cache instead of compiling it. The first time, Nix asks whether to trust the cache settings in the flake. Answer `y`, or pass `--accept-flake-config` to skip the prompt.

Or add it to your system flake:

```nix
{
  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-26.05";
  inputs.ollama-tray.url = "github:stephenbrannen/ollama-tray-go";
  inputs.ollama-tray.inputs.nixpkgs.follows = "nixpkgs";

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

* To use the cache for every build on the system, add it to your NixOS configuration. Without this, a system rebuild compiles the package, because only the flake's own settings trust the cache:

  ```nix
  nix.settings = {
    substituters = [ "https://ollama-tray.cachix.org" ];
    trusted-public-keys = [ "ollama-tray.cachix.org-1:VsA+mvlRdugWzqzTzrRhqRpJ25Sc9tbL6z4or9oHc+U=" ];
  };
  ```

How the three ways to get nixpkgs differ:

* `nix run github:...` uses the nixpkgs pinned in this repository's `flake.lock`. On x86_64-linux and aarch64-darwin it downloads the prebuilt program from the binary cache, about 10 MiB. On other systems, or if you decline the cache, Nix builds it from source. On Linux that downloads about 215 MiB (742 MiB unpacked) of build tools on an empty store, mainly Go and GCC.
* `inputs.ollama-tray.inputs.nixpkgs.follows = "nixpkgs"` builds the tray with your nixpkgs, so it shares your Go and libraries and adds no second nixpkgs to your system. The tray then needs your nixpkgs to have Go 1.26 or newer.
* `--override-input nixpkgs nixpkgs` does the same for a single command, such as `nix run github:stephenbrannen/ollama-tray-go --override-input nixpkgs nixpkgs`, without editing a flake.

To skip the build, use the release archive in the next section. It holds a prebuilt binary, so it needs neither Nix nor Go.

## Install from a release

Each tagged release has a `tar.gz` archive per architecture (`linux_amd64`, `linux_arm64`) and a `checksums.txt`.

* Download the archive for your architecture and `checksums.txt` from the Releases page, or with `gh release download <tag> -R stephenbrannen/ollama-tray-go -p '*_linux_amd64.tar.gz' -p checksums.txt` (use `arm64` on ARM).
* Verify it with `sha256sum -c checksums.txt --ignore-missing`.
* Verify its build provenance with `gh attestation verify <archive> -R stephenbrannen/ollama-tray-go`. This proves the archive came from this repository's release workflow. Releases from v0.1.2 on carry one; v0.1.0 and v0.1.1 were published while the repository was private and have none.
* Extract it with `tar xzf <archive>`, then move `ollama-tray` to `~/.local/bin`.
* Copy `ollama-tray.desktop` to `~/.config/autostart/` to start the tray on login. It runs `ollama-tray` from your `PATH`.

### Withdraw a bad release

* Delete the release and its tag with `gh release delete <tag> --cleanup-tag -R stephenbrannen/ollama-tray-go`.
* Fix the problem on `main`, then tag a patch version (for example `v0.1.2`). Do not reuse the deleted tag.

The release workflow runs `go vet` and the tests, and refuses a tag whose commit is not on `main`.

## Autostart

On Linux the package installs `ollama-tray.desktop` under `share/applications`. To start the tray on login, copy or link it into `~/.config/autostart/`. For macOS, see [Start at login](#start-at-login).

## macOS

On macOS the tray icon sits in the menu bar. The macOS tray library draws it through Cocoa, which Go can only reach with cgo, so every macOS build needs a C compiler. Linux builds do not.

Run it with Nix:

```sh
nix run github:stephenbrannen/ollama-tray-go
```

On Apple Silicon, Nix downloads the prebuilt package from the `ollama-tray` Cachix cache. On an Intel Mac, Nix builds it from source.

Run it from source with Go 1.26 or newer and the Xcode Command Line Tools (`xcode-select --install`), which provide the C compiler:

```sh
go run github.com/stephenbrannen/ollama-tray-go/cmd/ollama-tray@latest
```

`go install github.com/stephenbrannen/ollama-tray-go/cmd/ollama-tray@latest` puts the binary in `~/go/bin` instead.

### Start at login

* Login Items (System Settings, General, Login Items) accepts the binary, but macOS then opens it in a Terminal window.
* A LaunchAgent starts it with no window. Save this as `~/Library/LaunchAgents/com.github.stephenbrannen.ollama-tray.plist`, with the full path to your binary:

  ```xml
  <?xml version="1.0" encoding="UTF-8"?>
  <!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
  <plist version="1.0">
  <dict>
    <key>Label</key>
    <string>com.github.stephenbrannen.ollama-tray</string>
    <key>ProgramArguments</key>
    <array>
      <string>/Users/you/go/bin/ollama-tray</string>
      <string>--host</string>
      <string>http://127.0.0.1:11434</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
  </dict>
  </plist>
  ```

  Load it with `launchctl bootstrap gui/$(id -u) ~/Library/LaunchAgents/com.github.stephenbrannen.ollama-tray.plist`. A LaunchAgent does not read your shell profile, so set the server with `--host` instead of `OLLAMA_HOST`.

The end-to-end test runs on Linux only. It fakes the D-Bus tray host that Linux desktops use, and macOS has no such host.

## Development

* `nix develop` provides go and gopls on every system the flake builds for. golangci-lint and govulncheck are `tool` directives in `go.mod`, so run them as `go tool golangci-lint` and `go tool govulncheck`.
* `git config core.hooksPath .githooks` enables the hooks. `pre-commit` runs gofmt, `go mod tidy -diff`, `go vet`, golangci-lint, and govulncheck. `pre-push` runs the coverage gate and the `tasks` tests.
* `go run -tags task ./tasks/coverage.go` runs `go test -race -count=1 ./...` and fails below 80% statement coverage. The count leaves out `cmd/ollama-tray` and `internal/tray`, which are startup and tray wiring. `go test -tags task ./tasks/` tests that program.
* `GOOS=linux CGO_ENABLED=0 go build ./...` checks the Linux build. On a Mac, `CGO_ENABLED=1 go build ./cmd/ollama-tray` checks the macOS build.
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
