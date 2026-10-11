// Package bootstrap loads the application configuration, opens the shared
// infrastructure (logger, database, authentication), builds the router and
// runs the HTTP server.
package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/otyang/demo/internal/middleware"
	"github.com/otyang/demo/pkg/config"
	"github.com/otyang/demo/pkg/jwtutils"
	"github.com/otyang/demo/pkg/logger"
	"github.com/otyang/demo/pkg/store"
)

const (
	readHeaderTimeout = 5 * time.Second
	// readTimeout bounds reading a whole request, so a slow client cannot
	// hold a request body open; writeTimeout bounds the whole response.
	readTimeout     = 15 * time.Second
	writeTimeout    = 30 * time.Second
	idleTimeout     = 60 * time.Second
	shutdownTimeout = 10 * time.Second
)

// Infrastructure holds the shared application resources initialized at startup.
type Infrastructure struct {
	Config *AppConfig
	Logger *slog.Logger
	Store  *store.Store
	// Authenticate puts the caller's JWTClaims in the request context:
	// middleware.JWTAuthentication, or middleware.TestClaims when
	// DEMO_AUTH_TEST_CLAIMS is enabled. Mount applies it to every route
	// that requires a permission.
	Authenticate func(http.Handler) http.Handler

	// stopKeyRefresh ends the JWKS cache's background refresh.
	stopKeyRefresh context.CancelFunc
}

// NewInfrastructure loads the configuration from envFile and the
// environment, and initializes logging, authentication and the database.
// ctx only bounds startup; call Close once the server has stopped.
func NewInfrastructure(ctx context.Context, envFile string) (*Infrastructure, error) {
	cfg, err := config.Load[AppConfig](EnvPrefix, envFile)
	if err != nil {
		return nil, fmt.Errorf("load configuration: %w", err)
	}
	if err := validateConfig(&cfg); err != nil {
		return nil, err
	}

	logLevel, err := logger.ParseLevel(cfg.LogLevel)
	if err != nil {
		return nil, fmt.Errorf("parse configured log level: %w", err)
	}
	logFormat := logger.FormatJSON
	if cfg.Debug {
		logFormat = logger.FormatColor
	}
	appLogger, err := logger.New(logger.Options{
		Format:    logFormat,
		Level:     logLevel,
		AddSource: cfg.Debug,
	})
	if err != nil {
		return nil, fmt.Errorf("create application logger: %w", err)
	}
	// httpx logs request errors with slog.Default().
	if err := logger.SetDefault(appLogger); err != nil {
		return nil, fmt.Errorf("set default logger: %w", err)
	}

	// The key cache must outlive ctx (the shutdown-signal context), so
	// requests still running during shutdown can be authenticated.
	keyCtx, stopKeyRefresh := context.WithCancel(context.WithoutCancel(ctx))
	authenticate, err := newAuthentication(keyCtx, &cfg, appLogger)
	if err != nil {
		stopKeyRefresh()
		return nil, err
	}

	poolSize := cfg.Database.PoolSize
	if cfg.Database.Type == store.SQLite {
		// SQLite allows one writer at a time; a single connection avoids
		// "database is locked" errors under concurrent requests.
		poolSize = 1
	}
	database, err := store.New(store.Options{
		Type:         cfg.Database.Type,
		DSN:          cfg.Database.URL,
		MaxOpenConns: poolSize,
		MaxIdleConns: poolSize,
		Timeout:      cfg.Database.Timeout,
		LogQueries:   cfg.Debug,
		Logger:       appLogger,
	})
	if err != nil {
		stopKeyRefresh()
		return nil, fmt.Errorf("open database: %w", err)
	}

	return &Infrastructure{
		Config:         &cfg,
		Logger:         appLogger,
		Store:          database,
		Authenticate:   authenticate,
		stopKeyRefresh: stopKeyRefresh,
	}, nil
}

// validateConfig rejects settings that would make the service unsafe or
// unable to run.
func validateConfig(cfg *AppConfig) error {
	switch cfg.Database.Type {
	case store.SQLite:
	case store.Postgres:
		// Migrations hold one connection for their lock and need another to run.
		if cfg.Database.PoolSize < 2 {
			return fmt.Errorf("DEMO_DB_POOL_SIZE must be at least 2 with postgres")
		}
	default:
		return fmt.Errorf("DEMO_DB_TYPE must be sqlite or postgres, not %q", cfg.Database.Type)
	}
	if cfg.Auth.TestClaims {
		if !cfg.Debug {
			return fmt.Errorf("DEMO_AUTH_TEST_CLAIMS requires DEMO_DEBUG=true; never enable it in production")
		}
		return nil
	}
	// Without an issuer and audience, any token signed by a key in the JWKS
	// would be accepted, whoever it was issued for.
	for name, value := range map[string]string{
		"DEMO_AUTH_JWKS_URL": cfg.Auth.JWKSURL,
		"DEMO_AUTH_ISSUER":   cfg.Auth.Issuer,
		"DEMO_AUTH_AUDIENCE": cfg.Auth.Audience,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s is required unless DEMO_AUTH_TEST_CLAIMS is enabled", name)
		}
	}
	return nil
}

// newAuthentication returns the authentication middleware for cfg. ctx
// bounds the JWT verifier's key cache.
func newAuthentication(ctx context.Context, cfg *AppConfig, appLogger *slog.Logger) (func(http.Handler) http.Handler, error) {
	if cfg.Auth.TestClaims {
		appLogger.Warn("⚠️ JWT authentication is disabled: every request uses fixed test claims",
			"org_id", cfg.Auth.TestOrgID, "permissions", cfg.Auth.TestPermissions)
		return middleware.TestClaims(middleware.JWTClaims{
			Issuer:         "demo-test-claims",
			Subject:        "test-user",
			OrganizationID: cfg.Auth.TestOrgID,
			Email:          "test-user@example.com",
			Permissions:    cfg.Auth.TestPermissions,
		}), nil
	}
	verifier, err := jwtutils.NewJWKSVerifier(ctx, cfg.Auth.JWKSURL, cfg.Auth.Issuer, cfg.Auth.Audience)
	if err != nil {
		return nil, fmt.Errorf("create JWT verifier: %w", err)
	}
	return middleware.JWTAuthentication(verifier), nil
}

// Close releases the infrastructure. Call it after the HTTP server has stopped.
func (i *Infrastructure) Close() error {
	if i == nil {
		return nil
	}
	if i.stopKeyRefresh != nil {
		i.stopKeyRefresh()
	}
	if err := i.Store.Close(); err != nil {
		return fmt.Errorf("close infrastructure database: %w", err)
	}
	return nil
}

// Bootstrap runs an HTTP server until ctx is canceled, then attempts a
// graceful shutdown. Requests inherit ctx's values but not its cancellation,
// so requests in flight when shutdown starts can finish.
func Bootstrap(ctx context.Context, name, addr string, handler http.Handler, appLogger *slog.Logger) error {
	if ctx == nil {
		return fmt.Errorf("bootstrap HTTP server: context cannot be nil")
	}
	if handler == nil {
		return fmt.Errorf("bootstrap HTTP server: handler cannot be nil")
	}
	if appLogger == nil {
		appLogger = slog.Default()
	}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen on %q: %w", addr, err)
	}

	return serve(ctx, name, newServer(ctx, addr, handler), listener, appLogger)
}

// newServer builds the HTTP server. Request contexts carry ctx's values but
// not its cancellation: cancelling ctx starts a graceful shutdown, and
// requests already running must be allowed to finish.
func newServer(ctx context.Context, addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
		BaseContext: func(net.Listener) context.Context {
			return context.WithoutCancel(ctx)
		},
	}
}

func serve(ctx context.Context, name string, server *http.Server, listener net.Listener, appLogger *slog.Logger) error {
	if appLogger == nil {
		appLogger = slog.Default()
	}
	serverErrors := make(chan error, 1)
	go func() {
		appLogger.Info("HTTP server listening", "name", name, "address", listener.Addr().String())
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrors <- fmt.Errorf("HTTP server %q: %w", name, err)
			return
		}
		serverErrors <- nil
	}()

	select {
	case err := <-serverErrors:
		return err
	case <-ctx.Done():
		appLogger.Info("shutting down HTTP server", "name", name)
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			appLogger.Error("graceful HTTP shutdown failed", "name", name, "error", err)
			if closeErr := server.Close(); closeErr != nil {
				return fmt.Errorf("force close HTTP server %q after shutdown error %v: %w", name, err, closeErr)
			}
		}
		if err := <-serverErrors; err != nil {
			return err
		}
		return nil
	}
}
