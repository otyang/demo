package store

import (
	"bytes"
	"context"
	"embed"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/uptrace/bun"
)

type migrationRecord struct {
	bun.BaseModel `bun:"table:store_migration_records"`

	ID   int64  `bun:",pk,autoincrement"`
	Name string `bun:",notnull"`
}

//go:embed testdata/migrations/*
var embeddedMigrations embed.FS

func TestMigrateModels(t *testing.T) {
	database := newTestStore(t)
	var logOutput bytes.Buffer
	options := MigrationOptions{
		Logger: slog.New(slog.NewTextHandler(&logOutput, nil)),
	}

	if err := MigrateModels(
		context.Background(),
		database.BunDB,
		options,
		(*migrationRecord)(nil),
	); err != nil {
		t.Fatalf("MigrateModels() error = %v", err)
	}
	if err := MigrateModels(
		context.Background(),
		database.BunDB,
		options,
		(*migrationRecord)(nil),
	); err != nil {
		t.Fatalf("MigrateModels() second run error = %v", err)
	}

	assertTableExists(t, database, "store_migration_records")
	if !strings.Contains(logOutput.String(), "✅") {
		t.Errorf("migration logs %q do not contain success marker", logOutput.String())
	}
}

func TestMigrateDirectory(t *testing.T) {
	database := newTestStore(t)
	directory := t.TempDir()
	writeSQLMigration(t, directory)
	var logOutput bytes.Buffer

	if err := MigrateDirectory(
		context.Background(),
		database.BunDB,
		directory,
		MigrationOptions{Logger: slog.New(slog.NewTextHandler(&logOutput, nil))},
	); err != nil {
		t.Fatalf("MigrateDirectory() error = %v", err)
	}
	assertTableExists(t, database, "directory_migration_records")
	if !strings.Contains(logOutput.String(), "✅") {
		t.Errorf("migration logs %q do not contain success marker", logOutput.String())
	}
}

func TestMigrateEmbedded(t *testing.T) {
	database := newTestStore(t)
	if err := MigrateEmbedded(
		context.Background(),
		database.BunDB,
		embeddedMigrations,
		"testdata/migrations",
		MigrationOptions{},
	); err != nil {
		t.Fatalf("MigrateEmbedded() error = %v", err)
	}
	assertTableExists(t, database, "embedded_migration_records")
}

func TestMigrationHelpersReportInvalidInput(t *testing.T) {
	database := newTestStore(t)
	ctx := context.Background()

	for name, migrate := range map[string]func() error{
		"nil model database": func() error {
			return MigrateModels(ctx, nil, MigrationOptions{}, (*migrationRecord)(nil))
		},
		"no models": func() error {
			return MigrateModels(ctx, database.BunDB, MigrationOptions{})
		},
		"empty migration directory": func() error {
			return MigrateDirectory(ctx, database.BunDB, " ", MigrationOptions{})
		},
		"missing migration directory": func() error {
			return MigrateDirectory(ctx, database.BunDB, filepath.Join(t.TempDir(), "missing"), MigrationOptions{})
		},
		"nil embedded filesystem": func() error {
			return MigrateEmbedded(ctx, database.BunDB, nil, "migrations", MigrationOptions{})
		},
		"missing embedded directory": func() error {
			return MigrateEmbedded(ctx, database.BunDB, embeddedMigrations, "missing", MigrationOptions{})
		},
		"nil migration database": func() error {
			return MigrateFS(ctx, nil, embeddedMigrations, MigrationOptions{})
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := migrate(); err == nil {
				t.Fatal("migration helper error = nil, want error")
			}
		})
	}
}

func newTestStore(t *testing.T) *Store {
	t.Helper()
	database, err := New(Options{
		Type:         SQLite,
		DSN:          ":memory:",
		MaxOpenConns: 1,
		MaxIdleConns: 1,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("close test database: %v", err)
		}
	})
	return database
}

func assertTableExists(t *testing.T, database *Store, table string) {
	t.Helper()
	var count int
	err := database.SQLDB.QueryRowContext(
		context.Background(),
		"SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?",
		table,
	).Scan(&count)
	if err != nil {
		t.Fatalf("check table %q: %v", table, err)
	}
	if count != 1 {
		t.Errorf("table %q exists = %t, want true", table, count == 1)
	}
}

func writeSQLMigration(t *testing.T, directory string) {
	t.Helper()
	up := `CREATE TABLE directory_migration_records (id INTEGER PRIMARY KEY);`
	down := `DROP TABLE directory_migration_records;`
	for name, contents := range map[string]string{
		"001_create_directory_records.up.sql":   up,
		"001_create_directory_records.down.sql": down,
	} {
		if err := os.WriteFile(filepath.Join(directory, name), []byte(contents), 0o600); err != nil {
			t.Fatalf("write migration file %q: %v", name, err)
		}
	}
}

func TestMigrateFSRetriesFailedMigration(t *testing.T) {
	database := newTestStore(t)
	ctx := context.Background()
	options := MigrationOptions{
		Logger:    slog.New(slog.DiscardHandler),
		TableName: "retry_migrations",
	}
	failing := fstest.MapFS{
		"001_broken.up.sql": {Data: []byte("THIS IS NOT SQL;")},
	}
	if err := MigrateFS(ctx, database.BunDB, failing, options); err == nil {
		t.Fatal("MigrateFS() error = nil, want failure from invalid SQL")
	}

	fixed := fstest.MapFS{
		"001_broken.up.sql": {Data: []byte("CREATE TABLE IF NOT EXISTS retry_records (id INTEGER);")},
	}
	if err := MigrateFS(ctx, database.BunDB, fixed, options); err != nil {
		t.Fatalf("MigrateFS() retry error = %v", err)
	}
	assertTableExists(t, database, "retry_records")
	assertTableExists(t, database, "retry_migrations")

	var applied int
	if err := database.BunDB.NewSelect().Table("retry_migrations").ColumnExpr("COUNT(*)").Scan(ctx, &applied); err != nil {
		t.Fatalf("count applied migrations: %v", err)
	}
	if applied != 1 {
		t.Errorf("applied migrations = %d, want 1 (the failed attempt must not be recorded)", applied)
	}
}
