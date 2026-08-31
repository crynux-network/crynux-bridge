## Migration Authoring Rules

### Versioning Contract

Migrations are versioned and executed once by the migration framework.

Migration code in this directory MUST NOT add defensive re-execution guards such as:

- `HasTable` checks before `CreateTable` or `DropTable`
- `HasColumn` checks before `AddColumn` or `DropColumn`
- `HasIndex` checks before `CreateIndex` or `DropIndex`

Write migration steps as direct, deterministic schema transitions for the target version.

### No Unrequested Data Backfill

Migrations MUST NOT include data backfill logic unless the task explicitly requires it. When a migration creates a new table or column, leave it empty and let the application populate it, unless an explicit requirement states that historical data must be backfilled.

When backfill is explicitly required, `inference_tasks` and `events` MUST NOT be read with an unfiltered full-table query. Use selective filters and batched reads instead.

### Local Structs Only

Migration code MUST NOT reference structs from the `models` package. Define a local struct inside the migration file, frozen to the schema at the time the migration is written, with a `TableName()` method for the target table.

Live `models` structs keep evolving after a migration ships. A migration that references them produces a different schema every time the models change, which breaks replaying the migration chain on a fresh database.

Migration structs MUST NOT use GORM soft delete unless it is explicitly required. Do not embed `gorm.Model` and do not add a `gorm.DeletedAt` / `deleted_at` column. When a table needs ID and timestamps, define `ID`, `CreatedAt`, and `UpdatedAt` fields explicitly. This matches the model rule in `models/AGENTS.md`.

### GORM and Migration Library Versions

Use these versions when writing migrations:
- `go-gormigrate` `v2.1.0`
- `gorm` `v1.25.2`

Before editing a migration, check the official documentation for these exact versions to confirm the correct syntax and APIs. Do not hand-write SQL unless there is no supported GORM-based approach.

### Target DB

The online system is using MySQL v8.1.0 as the DB server. Make sure the migrations are compatible with it.

### Mandatory Local MySQL Testing

Every new or modified migration MUST be tested locally against a MySQL 8.1.0 instance before it is considered complete. SQLite-based unit tests do not satisfy this requirement.

The test MUST run the full migration chain from `migrate.InitMigration` on a clean, empty database, then roll back the newest migration and re-apply it. The migration is complete only when all of these runs succeed.
