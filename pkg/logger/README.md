# Logger

`github.com/otyang/demo/pkg/logger` wraps the Go standard library's `log/slog` with configurable
text, colored text or JSON output, severity filtering, source locations, process ID, version,
and build metadata.

## Development preset

Use colored text output, debug level, and source locations while developing:

```go
package main

import (
	"log/slog"
	"os"

	"github.com/otyang/demo/pkg/logger"
)

func main() {
	log, err := logger.NewDevelopment(logger.Options{
		Output:  os.Stderr,
		AddPID:  true,
		Version: "1.2.3",
		Build:   "local",
	})
	if err != nil {
		panic(err)
	}
	if err := logger.SetDefault(log); err != nil {
		panic(err)
	}

	slog.Debug("debug details are visible in development", "user_id", 42)
	slog.Info("application started")
}
```

After calling `SetDefault`, package-level `slog.Debug`, `slog.Info`,
`slog.Warn`, and `slog.Error` calls use the configured logger.

The development preset writes `logger.FormatColor`, rendered by
[tint](https://github.com/lmittmann/tint): colored, abbreviated levels
(`DBG`, `INF`, `WRN`, `ERR`), the source file and line, and colored keys:

```text
2026-10-09 01:24:03 ERR httpx/handler.go:92 request failed pid=41031 status=500 error_code=DATABASE_ERROR
```

Colors are only written to a terminal. When the output is a file, a pipe or
another writer, or the `NO_COLOR` environment variable is set, the same layout
is written without escape codes. The services use this format when
`NORO_DEBUG=true`, and JSON otherwise.

## Production preset

The production preset selects JSON output at info level without source
locations:

```go
log, err := logger.NewProduction(logger.Options{
	AddPID:  true,
	Version: version.Value,
	Build:   buildID,
})
if err != nil {
	return err
}
if err := logger.SetDefault(log); err != nil {
	return err
}
```

Both presets accept an optional `Output` writer, `AddPID`, `Version`, and
`Build`. If `Output` is nil, logs go to standard error.

## Custom options

Use `New` when a preset does not match the application. Supported formats are
`logger.FormatText` (`slog`'s key=value text), `logger.FormatColor` and
`logger.FormatJSON`. A zero `Level` means info level.
`ParseLevel` maps environment/config strings to `slog.Level`:

```go
level, err := logger.ParseLevel(cfg.LogLevel)
if err != nil {
	return err
}

log, err := logger.New(logger.Options{
	Format:    logger.FormatJSON,
	Level:     level,
	AddSource: true,
	Output:    os.Stdout,
	AddPID:    true,
	Version:   version.Value,
	Build:     buildID,
})
if err != nil {
	return err
}
log.Info("server started", "address", cfg.ServerHost)
```

`ParseLevel` accepts `debug`, `info`, `warn`/`warning`, and `error`.
