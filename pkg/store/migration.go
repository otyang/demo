package store

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"strings"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect"
	"github.com/uptrace/bun/migrate"
)

// MigrationOptions configures SQL migrations.
type MigrationOptions struct {
	Logger *slog.Logger
	// TableName records applied migrations; it defaults to Bun's
	// "bun_migrations". Give each independently migrated schema its own table
	// so migration IDs from different services cannot collide. A matching
	// "<TableName>_locks" table is used for Bun's lock bookkeeping.
	TableName string
}

// MigrateModels creates any missing tables described by the provided Bun
// models. It does not alter or drop existing tables or columns.
func MigrateModels(ctx context.Context, db *bun.DB, options MigrationOptions, models ...any) error {
	logger := migrationLogger(options.Logger)
	if db == nil {
		return migrationError(logger, "model", fmt.Errorf("database cannot be nil"))
	}
	if len(models) == 0 {
		return migrationError(logger, "model", fmt.Errorf("at least one model is required"))
	}

	logger.Info("🛠️ Starting model migration", "models", len(models))
	for _, model := range models {
		if model == nil {
			return migrationError(logger, "model", fmt.Errorf("model cannot be nil"))
		}
		if _, err := db.NewCreateTable().Model(model).IfNotExists().Exec(ctx); err != nil {
			return migrationError(logger, "model", fmt.Errorf("create model table: %w", err))
		}
	}
	logger.Info("✅ Model migration complete", "models", len(models))
	return nil
}

// MigrateDirectory discovers and applies Bun SQL migrations from a directory.
// Files must be named <numeric-id>_<description>.up.sql and
// <numeric-id>_<description>.down.sql.
func MigrateDirectory(
	ctx context.Context,
	db *bun.DB,
	directory string,
	options MigrationOptions,
) error {
	if strings.TrimSpace(directory) == "" {
		return migrationError(migrationLogger(options.Logger), "directory", fmt.Errorf("migration directory cannot be empty"))
	}
	info, err := os.Stat(directory)
	if err != nil {
		return migrationError(migrationLogger(options.Logger), "directory", fmt.Errorf("stat migration directory: %w", err))
	}
	if !info.IsDir() {
		return migrationError(migrationLogger(options.Logger), "directory", fmt.Errorf("migration path %q is not a directory", directory))
	}
	return MigrateFS(ctx, db, os.DirFS(directory), options)
}

// MigrateEmbedded discovers and applies Bun SQL migrations from a subdirectory
// within an embedded filesystem.
func MigrateEmbedded(
	ctx context.Context,
	db *bun.DB,
	filesystem fs.FS,
	directory string,
	options MigrationOptions,
) error {
	logger := migrationLogger(options.Logger)
	if filesystem == nil {
		return migrationError(logger, "embedded", fmt.Errorf("embedded filesystem cannot be nil"))
	}
	if strings.TrimSpace(directory) == "" {
		return migrationError(logger, "embedded", fmt.Errorf("embedded migration directory cannot be empty"))
	}
	subFilesystem, err := fs.Sub(filesystem, directory)
	if err != nil {
		return migrationError(logger, "embedded", fmt.Errorf("open embedded migration directory %q: %w", directory, err))
	}
	return MigrateFS(ctx, db, subFilesystem, options)
}

// MigrateFS discovers and applies Bun SQL migrations from any fs.FS. This
// supports embedded migrations and other virtual filesystems.
func MigrateFS(ctx context.Context, db *bun.DB, filesystem fs.FS, options MigrationOptions) error {
	logger := migrationLogger(options.Logger)
	if db == nil {
		return migrationError(logger, "filesystem", fmt.Errorf("database cannot be nil"))
	}
	if filesystem == nil {
		return migrationError(logger, "filesystem", fmt.Errorf("migration filesystem cannot be nil"))
	}

	migrations := migrate.NewMigrations()
	if err := migrations.Discover(filesystem); err != nil {
		return migrationError(logger, "filesystem", fmt.Errorf("discover SQL migrations: %w", err))
	}
	found := migrations.Sorted()
	if len(found) == 0 {
		logger.Info("✅ No SQL migrations found")
		return nil
	}

	logger.Info("🛠️ Starting SQL migration", "migrations", len(found))
	// Record a migration only after it succeeds, so a failed or cancelled
	// migration is retried on the next run instead of being skipped.
	migratorOptions := []migrate.MigratorOption{migrate.WithMarkAppliedOnSuccess(true)}
	lockKey := "bun_migrations"
	if tableName := strings.TrimSpace(options.TableName); tableName != "" {
		lockKey = tableName
		migratorOptions = append(migratorOptions,
			migrate.WithTableName(tableName),
			migrate.WithLocksTableName(tableName+"_locks"),
		)
	}
	migrator := migrate.NewMigrator(db, migrations, migratorOptions...)

	var group *migrate.MigrationGroup
	err := withMigrationLock(ctx, db, lockKey, func() error {
		if err := migrator.Init(ctx); err != nil {
			return fmt.Errorf("initialize migration tables: %w", err)
		}
		var err error
		if group, err = migrator.Migrate(ctx); err != nil {
			return fmt.Errorf("apply SQL migrations: %w", err)
		}
		return nil
	})
	if err != nil {
		return migrationError(logger, "filesystem", err)
	}
	logger.Info("✅ SQL migration complete", "applied", len(group.Migrations))
	return nil
}

// withMigrationLock serializes migrations across processes. On PostgreSQL it
// holds a session advisory lock, which waits for other migrators and is
// released automatically if the process dies. Other dialects run fn directly.
func withMigrationLock(ctx context.Context, db *bun.DB, key string, fn func() error) error {
	if db.Dialect().Name() != dialect.PG {
		return fn()
	}
	// The lock holds one pooled connection for the whole run; the migrations
	// need another. With a single-connection pool they would wait forever.
	if db.Stats().MaxOpenConnections == 1 {
		return fmt.Errorf("migrations need a connection pool of at least 2 (the lock holds one connection)")
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("acquire migration lock connection: %w", err)
	}
	defer conn.Close()

	if _, err := conn.ExecContext(ctx, "SELECT pg_advisory_lock(hashtext(?))", key); err != nil {
		return fmt.Errorf("acquire migration lock: %w", err)
	}
	defer func() {
		// Use a fresh context so the lock is released even if ctx was cancelled.
		_, _ = conn.ExecContext(context.Background(), "SELECT pg_advisory_unlock(hashtext(?))", key)
	}()
	return fn()
}

func migrationLogger(logger *slog.Logger) *slog.Logger {
	if logger == nil {
		return slog.Default()
	}
	return logger
}

func migrationError(logger *slog.Logger, source string, err error) error {
	logger.Error("❌ Database migration failed", "source", source, "error", err)
	return err
}
