CREATE TABLE IF NOT EXISTS tasks (
    id TEXT PRIMARY KEY,
    organisation_id TEXT NOT NULL,
    title TEXT NOT NULL,
    notes TEXT NOT NULL DEFAULT '',
    done BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL
);

--bun:split

CREATE INDEX IF NOT EXISTS tasks_organisation_created_idx ON tasks (organisation_id, created_at, id);
