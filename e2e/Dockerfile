# The test container: a session D-Bus, the tray built from this checkout, and the compiled e2e test.
FROM golang:1.26.9-trixie@sha256:f06b7122f888680048afb247c4e5f49c0069eb6c4c1ccae0f3f4834527a8d22c AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY e2e ./e2e
# fyne.io/systray speaks D-Bus in pure Go on Linux, so neither binary needs cgo or an X display.
RUN CGO_ENABLED=0 go build -o /out/ollama-tray ./cmd/ollama-tray \
    && CGO_ENABLED=0 go test -c -tags e2e -o /out/e2e.test ./e2e

FROM debian:13.7-slim@sha256:a29215f6a35e51e22adffa17f89e9d2ef06214e64a2bad10d765c46aea49f11f
RUN apt-get update \
    && apt-get install -y --no-install-recommends dbus-daemon \
    && rm -rf /var/lib/apt/lists/*
COPY --from=build /out/ollama-tray /out/e2e.test /usr/local/bin/
ENV OLLAMA_TRAY=/usr/local/bin/ollama-tray
# `dbus-run-session` gives the run its own session bus and stops it when the test exits.
ENTRYPOINT ["dbus-run-session", "--", "/usr/local/bin/e2e.test", "-test.v"]
