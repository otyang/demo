// Package store creates and manages database handles used with Bun.
package store

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/pgdriver"
	"github.com/uptrace/bun/driver/sqliteshim"
	"github.com/uptrace/bun/extra/bunslog"
)

type DatabaseType string

const (
	Postgres DatabaseType = "postgres"
	SQLite   DatabaseType = "sqlite"

	defaultTimeout = 5 * time.Second
)

// Options configures the database connection and pool.
type Options struct {
	Type            DatabaseType
	DSN             string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	Timeout         time.Duration
	// LogQueries logs every query Bun runs, with its duration, at info
	// level (failed queries at error level). Use it in debug mode only: the
	// logged SQL includes its argument values.
	LogQueries bool
	// Logger receives the query logs; nil uses slog.Default().
	Logger *slog.Logger
}

// Store owns the Bun and database/sql handles for a shared connection pool.
type Store struct {
	BunDB *bun.DB
	SQLDB *sql.DB
}

// New opens a database, configures its connection pool, and verifies
// connectivity using Timeout. Close the returned Store to release the
// underlying connection pool.
func New(options Options) (*Store, error) {
	if strings.TrimSpace(options.DSN) == "" {
		return nil, fmt.Errorf("database DSN cannot be empty")
	}
	if options.MaxOpenConns < 0 {
		return nil, fmt.Errorf("max open connections cannot be negative")
	}
	if options.MaxIdleConns < 0 {
		return nil, fmt.Errorf("max idle connections cannot be negative")
	}
	if options.ConnMaxLifetime < 0 {
		return nil, fmt.Errorf("connection max lifetime cannot be negative")
	}
	if options.Timeout < 0 {
		return nil, fmt.Errorf("database timeout cannot be negative")
	}

	var (
		sqlDB *sql.DB
		bunDB *bun.DB
	)
	switch options.Type {
	case Postgres:
		sqlDB = sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(options.DSN)))
		bunDB = bun.NewDB(sqlDB, pgdialect.New(), bun.WithDiscardUnknownColumns())
	case SQLite:
		var err error
		sqlDB, err = sql.Open(sqliteshim.ShimName, options.DSN)
		if err != nil {
			return nil, fmt.Errorf("open sqlite database: %w", err)
		}
		bunDB = bun.NewDB(sqlDB, sqlitedialect.New())
	default:
		return nil, fmt.Errorf("unsupported database type %q", options.Type)
	}

	if options.LogQueries {
		logger := options.Logger
		if logger == nil {
			logger = slog.Default()
		}
		bunDB.AddQueryHook(bunslog.NewQueryHook(
			bunslog.WithLogger(logger),
			bunslog.WithQueryLogLevel(slog.LevelInfo),
		))
	}

	sqlDB.SetMaxOpenConns(options.MaxOpenConns)
	sqlDB.SetMaxIdleConns(options.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(options.ConnMaxLifetime)

	timeout := options.Timeout
	if timeout == 0 {
		timeout = defaultTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := bunDB.PingContext(ctx); err != nil {
		_ = bunDB.Close()
		return nil, fmt.Errorf("ping %s database: %w", options.Type, err)
	}
	return &Store{BunDB: bunDB, SQLDB: sqlDB}, nil
}

// Close closes the Bun database and its underlying sql.DB connection pool.
func (s *Store) Close() error {
	if s == nil || s.BunDB == nil {
		return nil
	}
	if err := s.BunDB.Close(); err != nil {
		return fmt.Errorf("close database: %w", err)
	}
	return nil
}

// GenerateCreateTableSQL returns the Bun-generated create-table statements
// for the provided models and dialect. It does not open or require a database
// connection. Each statement is idempotent with IF NOT EXISTS.
func GenerateCreateTableSQL(databaseType DatabaseType, models ...any) (string, error) {
	db, err := newBunDB(databaseType)
	if err != nil {
		return "", err
	}
	if len(models) == 0 {
		return "", fmt.Errorf("at least one model is required")
	}

	statements := make([]string, 0, len(models))
	for _, model := range models {
		if model == nil {
			return "", fmt.Errorf("model cannot be nil")
		}
		statement := db.NewCreateTable().Model(model).IfNotExists().String()
		if strings.TrimSpace(statement) == "" {
			return "", fmt.Errorf("generate create-table SQL: empty statement")
		}
		statements = append(statements, strings.TrimSuffix(strings.TrimSpace(statement), ";"))
	}
	return strings.Join(statements, ";\n") + ";", nil
}

func newBunDB(databaseType DatabaseType) (*bun.DB, error) {
	switch databaseType {
	case Postgres:
		return bun.NewDB(nil, pgdialect.New()), nil
	case SQLite:
		return bun.NewDB(nil, sqlitedialect.New()), nil
	default:
		return nil, fmt.Errorf("unsupported database type %q", databaseType)
	}
}
