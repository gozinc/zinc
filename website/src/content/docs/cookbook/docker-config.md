---
title: Docker and Environment Config
description: Read settings from environment variables with defaults, build a small container image, and shut down cleanly when the container stops.
---

This program reads its port, environment name, log level and shutdown timeout from environment variables, falling back to defaults when they're unset. It ships in a small container image and finishes in-flight requests when the container is stopped. You'd use this shape for any service deployed as a container, where settings come from the platform rather than from flags or files.

## The program

```go title="main.go"
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/0mjs/zinc"
	"github.com/0mjs/zinc/middleware/logger"
)

// config holds every setting the service reads from the environment.
type config struct {
	Addr            string
	Env             string
	LogLevel        slog.Level
	ShutdownTimeout time.Duration
}

// getenv returns the variable's value, or fallback when it's unset or empty.
func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func loadConfig() (config, error) {
	cfg := config{
		Addr: ":" + getenv("PORT", "8080"),
		Env:  getenv("APP_ENV", "development"),
	}
	if err := cfg.LogLevel.UnmarshalText([]byte(getenv("LOG_LEVEL", "info"))); err != nil {
		return cfg, fmt.Errorf("LOG_LEVEL: %w", err)
	}
	timeout, err := time.ParseDuration(getenv("SHUTDOWN_TIMEOUT", "10s"))
	if err != nil {
		return cfg, fmt.Errorf("SHUTDOWN_TIMEOUT: %w", err)
	}
	cfg.ShutdownTimeout = timeout
	return cfg, nil
}

func main() {
	cfg, err := loadConfig()
	if err != nil {
		slog.Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))

	app := zinc.New(zinc.Config{ShutdownTimeout: cfg.ShutdownTimeout})
	app.Use(logger.New(logger.Config{Logger: log}))

	app.Get("/", func(c *zinc.Context) error {
		return c.JSON(zinc.Map{"env": cfg.Env})
	})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Info("starting", "addr", cfg.Addr, "env", cfg.Env)
	if err := app.ListenContext(ctx, cfg.Addr); err != nil {
		log.Error("shutdown", "error", err)
		os.Exit(1)
	}
	log.Info("stopped")
}
```

## The Dockerfile

Put this next to `main.go`, `go.mod` and `go.sum`:

```dockerfile title="Dockerfile"
# Build stage: compile a static binary.
FROM golang:1.25 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/server .

# Run stage: only the binary, running as a non-root user.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/server /server
EXPOSE 8080
ENTRYPOINT ["/server"]
```

## Try it

Run it without Docker first. Settings come from the environment:

```bash
PORT=3000 APP_ENV=production go run .
```

```bash
curl http://localhost:3000/
# {"env":"production"}
```

Stop it with `kill -TERM <pid>` or Ctrl-C, and it logs the request and a clean stop:

```text
{"time":"2026-09-28T01:22:51.416903+01:00","level":"INFO","msg":"starting","addr":":3000","env":"production"}
{"time":"2026-09-28T01:22:52.070532+01:00","level":"INFO","msg":"REQUEST","method":"GET","uri":"/","route":"/","status":200,"latency":262958,"host":"localhost:3000","bytes_in":"","bytes_out":21,"user_agent":"curl/8.7.1","remote_ip":"::1","request_id":""}
{"time":"2026-09-28T01:22:52.092439+01:00","level":"INFO","msg":"stopped"}
```

A value that doesn't parse stops the program before it starts serving, with exit status 1:

```bash
SHUTDOWN_TIMEOUT=30 go run .
# 2026/09/28 01:24:57 ERROR invalid configuration error="SHUTDOWN_TIMEOUT: time: missing unit in duration \"30\""

LOG_LEVEL=loud go run .
# 2026/09/28 01:24:57 ERROR invalid configuration error="LOG_LEVEL: slog: level string \"loud\": unknown name"
```

Then build and run the image:

```bash
docker build -t zinc-app .
docker run --rm -p 8080:8080 -e APP_ENV=staging zinc-app
curl http://localhost:8080/
# {"env":"staging"}
docker stop <container>
```

## How it works

- `getenv` returns a default when a variable is unset or empty, so the program runs locally with no setup.
- `loadConfig` parses every value once at startup and returns an error naming the variable. A typo fails the deploy immediately instead of on the first request that needs the value.
- `Addr` is `":" + PORT`, which listens on every interface. Inside a container, `localhost:8080` would only accept connections from the container itself.
- `signal.NotifyContext` cancels on `SIGTERM`, which `docker stop` sends. `app.ListenContext` then stops accepting connections and waits up to `ShutdownTimeout` for requests in progress.
- The exec form, `ENTRYPOINT ["/server"]`, runs the binary directly, so it receives `SIGTERM` itself. The shell form (`ENTRYPOINT /server`) would start `/bin/sh`, which doesn't pass signals on, and the distroless image has no shell at all.

## Before production

- `docker stop` waits 10 seconds before sending `SIGKILL`, the same as Zinc's default `ShutdownTimeout`. Set `SHUTDOWN_TIMEOUT` a little lower, or give Docker longer with `docker stop -t 30` or `--stop-timeout`.
- Add a `.dockerignore` listing `.git` and local build output, so `COPY . .` doesn't send them to the build.
- Pass secrets as environment variables from your platform's secret store, and don't log the loaded config.
- `CGO_ENABLED=0` makes a static binary that the distroless image can run. A driver that needs cgo, such as `mattn/go-sqlite3`, needs a base image with a C library instead.

## See also

- [Graceful Shutdown](/cookbook/graceful-shutdown/): watch a request drain during shutdown.
- [Configuration](/guide/configuration/): every `zinc.Config` field, including the server timeouts.
- [Health and Readiness Checks](/cookbook/health-readiness/): endpoints for your orchestrator's probes.
