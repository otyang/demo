package logger

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/lmittmann/tint"
)

type Format string

const (
	FormatText Format = "text"
	FormatJSON Format = "json"
	// FormatColor is human-readable text with colored levels and values,
	// written by github.com/lmittmann/tint. Colors are left out when the
	// output is not a terminal or NO_COLOR is set (https://no-color.org).
	FormatColor Format = "color"
)

type Options struct {
	Format    Format
	Level     slog.Level
	AddSource bool
	Output    io.Writer
	AddPID    bool
	Version   string
	Build     string
}

func NewDevelopment(options Options) (*slog.Logger, error) {
	options.Format = FormatColor
	options.Level = slog.LevelDebug
	options.AddSource = true
	return New(options)
}

func NewProduction(options Options) (*slog.Logger, error) {
	options.Format = FormatJSON
	options.Level = slog.LevelInfo
	options.AddSource = false
	return New(options)
}

func SetDefault(logger *slog.Logger) error {
	if logger == nil {
		return fmt.Errorf("set default logger: logger is nil")
	}
	slog.SetDefault(logger)
	return nil
}

func New(options Options) (*slog.Logger, error) {
	format := options.Format
	if format == "" {
		format = FormatText
	}

	handlerOptions := &slog.HandlerOptions{
		AddSource: options.AddSource,
		Level:     options.Level,
	}

	output := outputOrDefault(options.Output)
	var handler slog.Handler
	switch format {
	case FormatText:
		handler = slog.NewTextHandler(output, handlerOptions)
	case FormatJSON:
		handler = slog.NewJSONHandler(output, handlerOptions)
	case FormatColor:
		handler = tint.NewTextHandler(output, &tint.Options{
			AddSource:  options.AddSource,
			Level:      options.Level,
			TimeFormat: time.DateTime,
			NoColor:    !colorSupported(output),
		})
	default:
		return nil, fmt.Errorf("unsupported log format %q", format)
	}

	logger := slog.New(handler)
	if options.AddPID {
		logger = logger.With("pid", os.Getpid())
	}
	if options.Version != "" {
		logger = logger.With("version", options.Version)
	}
	if options.Build != "" {
		logger = logger.With("build", options.Build)
	}
	return logger, nil
}

func ParseLevel(value string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "debug":
		return slog.LevelDebug, nil
	case "info", "":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("unsupported log level %q", value)
	}
}

// colorSupported reports whether output is a terminal and NO_COLOR is not
// set, so color escape codes are never written to files or pipes.
func colorSupported(output io.Writer) bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	file, ok := output.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func outputOrDefault(output io.Writer) io.Writer {
	if output == nil {
		return os.Stderr
	}
	return output
}
