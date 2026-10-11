package store

import (
	"context"
	"database/sql"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/driver/sqliteshim"
)

type testRecord struct {
	bun.BaseModel `bun:"table:store_test_records"`

	ID   int64  `bun:",pk,autoincrement"`
	Name string `bun:",notnull"`
}

type secondTestRecord struct {
	bun.BaseModel `bun:"table:store_test_records_second"`

	ID int64 `bun:",pk,autoincrement"`
}

func TestNewSQLiteAndClose(t *testing.T) {
	database, err := New(Options{
		Type:            SQLite,
		DSN:             ":memory:",
		MaxOpenConns:    1,
		MaxIdleConns:    1,
		ConnMaxLifetime: time.Minute,
		Timeout:         time.Second,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})

	if database.BunDB.Dialect().Name().String() != "sqlite" {
		t.Errorf("dialect = %q, want sqlite", database.BunDB.Dialect().Name())
	}
	if got := database.SQLDB.Stats().MaxOpenConnections; got != 1 {
		t.Errorf("max open connections = %d, want 1", got)
	}
	if err := database.SQLDB.PingContext(context.Background()); err != nil {
		t.Fatalf("raw sql database ping: %v", err)
	}
	if err := database.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := database.SQLDB.Ping(); err == nil {
		t.Error("raw sql DB ping after Close() succeeded, want closed database error")
	}
}

func TestGenerateCreateTableSQL(t *testing.T) {
	statement, err := GenerateCreateTableSQL(SQLite, (*testRecord)(nil))
	if err != nil {
		t.Fatalf("GenerateCreateTableSQL() error = %v", err)
	}
	lowerStatement := strings.ToLower(statement)
	if !strings.Contains(lowerStatement, "create table if not exists \"store_test_records\"") {
		t.Errorf("generated SQL = %q, want create table statement", statement)
	}
	if strings.HasSuffix(statement, ";;") {
		t.Errorf("generated SQL ends in duplicate semicolons: %q", statement)
	}
	sqlDB, err := sql.Open(sqliteshim.ShimName, ":memory:")
	if err != nil {
		t.Fatalf("open sqlite database: %v", err)
	}
	defer sqlDB.Close()
	if _, err := sqlDB.ExecContext(context.Background(), statement); err != nil {
		t.Fatalf("execute generated SQL: %v", err)
	}

	multipleStatements, err := GenerateCreateTableSQL(
		SQLite,
		(*testRecord)(nil),
		(*secondTestRecord)(nil),
	)
	if err != nil {
		t.Fatalf("GenerateCreateTableSQL() with multiple models error = %v", err)
	}
	if _, err := sqlDB.ExecContext(context.Background(), multipleStatements); err != nil {
		t.Fatalf("execute multiple generated statements: %v", err)
	}
	if _, err := GenerateCreateTableSQL(Postgres, (*testRecord)(nil)); err != nil {
		t.Fatalf("GenerateCreateTableSQL() postgres error = %v", err)
	}
}

func TestNewRejectsInvalidOptions(t *testing.T) {
	for name, options := range map[string]Options{
		"unsupported database": {Type: "mysql", DSN: "dsn"},
		"empty DSN":            {Type: SQLite},
		"negative pool":        {Type: SQLite, DSN: ":memory:", MaxOpenConns: -1},
		"negative timeout":     {Type: SQLite, DSN: ":memory:", Timeout: -time.Second},
		"sqlite ping failure":  {Type: SQLite, DSN: "file:" + t.TempDir() + "/missing-dir/db.sqlite"},
	} {
		t.Run(name, func(t *testing.T) {
			database, err := New(options)
			if err == nil {
				if database != nil {
					_ = database.Close()
				}
				t.Fatal("New() error = nil, want validation error")
			}
			if database != nil {
				t.Fatalf("New() store = %v, want nil on error", database)
			}
		})
	}
}

func TestCloseNilStore(t *testing.T) {
	var database *Store
	if err := database.Close(); err != nil {
		t.Errorf("(*Store)(nil).Close() error = %v, want nil", err)
	}
}

func TestGenerateCreateTableSQLRejectsInvalidArguments(t *testing.T) {
	if _, err := GenerateCreateTableSQL(SQLite); err == nil {
		t.Error("GenerateCreateTableSQL(SQLite) error = nil, want error")
	}
	if _, err := GenerateCreateTableSQL(SQLite, nil); err == nil {
		t.Error("GenerateCreateTableSQL(SQLite, nil) error = nil, want error")
	}
	if _, err := GenerateCreateTableSQL(DatabaseType("mysql"), (*testRecord)(nil)); err == nil {
		t.Error("GenerateCreateTableSQL(mysql, model) error = nil, want error")
	}
}

func TestNewLogsQueriesWhenAsked(t *testing.T) {
	for _, logQueries := range []bool{true, false} {
		var output strings.Builder
		database, err := New(Options{
			Type:         SQLite,
			DSN:          ":memory:",
			MaxOpenConns: 1,
			LogQueries:   logQueries,
			Logger:       slog.New(slog.NewTextHandler(&output, nil)),
		})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}
		var one int
		if err := database.BunDB.NewSelect().ColumnExpr("1").Scan(context.Background(), &one); err != nil {
			t.Fatalf("select: %v", err)
		}
		_ = database.Close()

		logged := strings.Contains(output.String(), "SELECT 1")
		if logged != logQueries {
			t.Errorf("LogQueries=%v: query logged = %v; output %q", logQueries, logged, output.String())
		}
	}
}
