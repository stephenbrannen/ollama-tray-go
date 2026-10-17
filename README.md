# ollama-tray-go

A Linux system tray icon that shows which models an Ollama server has loaded, and unloads them on demand.

## Development

* `nix develop` provides go, gopls, golangci-lint, and pre-commit (x86_64-linux and aarch64-linux).
* `pre-commit install` enables the gofmt, go vet, and golangci-lint hooks.
* `go test -race -count=1 ./...` runs the tests.
* `GOOS=linux CGO_ENABLED=0 go build ./...` checks the Linux build.
