# Store

`pkg/store` opens PostgreSQL or SQLite databases with Bun. `store.New`
returns a `*store.Store` that exposes both the `*bun.DB` and its underlying
`*sql.DB`:

```go
database, err := store.New(store.Options{
	Type:            store.Postgres,
	DSN:             os.Getenv("DB_URL"),
	MaxOpenConns:    10,
	MaxIdleConns:    5,
	ConnMaxLifetime: time.Hour,
	Timeout:         5 * time.Second,
})
if err != nil {
	return err
}
defer database.Close()

_ = database.SQLDB // Use the raw database/sql handle when needed.
_ = database.BunDB // Use Bun for queries.
```

Use `store.SQLite` for SQLite, with a SQLite DSN such as `./app.db` or
`:memory:`. SQLite's `:memory:` database is connection-local, so use
`MaxOpenConns: 1` for that DSN or use a shared-cache in-memory DSN when a larger
pool is needed. A zero timeout uses a five-second default. A zero maximum-open
limit means unlimited connections; a zero maximum-idle limit disables idle
connections. A zero connection lifetime leaves connections unbounded by age.
The Bun handle owns the raw handle, so close the pair with `database.Close()`.

Set `LogQueries: true` to log every query Bun runs through
[`bunslog`](https://pkg.go.dev/github.com/uptrace/bun/extra/bunslog): the
operation, the SQL with its arguments filled in, and the duration, at info
level (failed queries at error level), so the logger's level must be info or
lower to show them. `Logger` picks the `*slog.Logger`, and defaults to
`slog.Default()`. The services turn it on when `NORO_DEBUG=true`.
Never enable it in production: the logged SQL includes values such as emails.

`store.GenerateCreateTableSQL(store.SQLite, models...)` or
`store.GenerateCreateTableSQL(store.Postgres, models...)` returns idempotent
Bun `CREATE TABLE IF NOT EXISTS` statements for one or more models. SQL
generation only needs the dialect and model definitions; it does not open a
connection. Execute the returned SQL using `database.SQLDB` or `database.BunDB`
to initialize the schema.

## Migrations

Create missing tables from Bun model structs:

```go
err := store.MigrateModels(ctx, database.BunDB, store.MigrationOptions{
	Logger: slog.Default(),
}, (*beneficiary.Beneficiary)(nil))
```

This model helper is deliberately non-destructive: it creates tables that do
not exist, but does not alter or drop existing tables or columns.

For versioned SQL changes, create matching `.up.sql` and `.down.sql` files.
Names use Bun's format, such as `001_create_beneficiaries.up.sql` and
`001_create_beneficiaries.down.sql`. Apply migrations from a directory with
`store.MigrateDirectory(ctx, database.BunDB, "./migrations", options)`, or
from any filesystem with `store.MigrateFS`. `store.MigrateEmbedded` accepts an
`fs.FS` (for example, an `embed.FS`) and a directory within it.

A migration is recorded only after it succeeds, so a failed or interrupted
migration is retried on the next run. Name files `<id>_<name>.tx.up.sql` to
run a migration inside a transaction. Set `MigrationOptions.TableName` to give
each independently migrated schema its own history table (Bun's default is
`bun_migrations`), so migration IDs from different services cannot collide.
On PostgreSQL, runs are serialized with a session advisory lock keyed by the
table name: concurrent replicas wait for each other, and the lock is released
automatically if a process dies. The lock holds one pooled connection while
the migrations use another, so PostgreSQL migrations require a pool of at
least two connections.

All helpers use the supplied `slog.Logger`, or `slog.Default()` when omitted,
and emit start, success, and failure messages with emoji markers.
