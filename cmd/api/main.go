// Command api serves the demo REST API. Its reference is served apart, by
// the doc command (go run ./doc).
package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/otyang/demo/internal/api"
	"github.com/otyang/demo/internal/bootstrap"
)

func main() {
	envFile := flag.String("env", "", "env file to load (default .env, or .example.env when there is no .env)")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	infra, err := bootstrap.NewInfrastructure(ctx, chooseEnvFile(*envFile))
	if err != nil {
		slog.Error("initialize application infrastructure", "error", err)
		os.Exit(1)
	}

	if err := errors.Join(run(ctx, infra), infra.Close()); err != nil {
		slog.Error("application stopped", "error", err)
		os.Exit(1)
	}
}

// chooseEnvFile returns the env file to load: flagValue if set, else .env,
// else .example.env.
func chooseEnvFile(flagValue string) string {
	if flagValue != "" {
		return flagValue
	}
	if _, err := os.Stat(".env"); err == nil {
		return ".env"
	}
	return ".example.env"
}

// run serves the API until ctx is cancelled.
func run(ctx context.Context, infra *bootstrap.Infrastructure) error {
	router, err := api.NewRouter(ctx, infra)
	if err != nil {
		return err
	}
	return bootstrap.Bootstrap(ctx, infra.Config.AppName, infra.Config.Host, router, infra.Logger)
}
