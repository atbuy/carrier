package store

import (
	"database/sql"
	"embed"
	"strings"

	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

func migrate(db *sql.DB) error {
	goose.SetBaseFS(migrationFS)
	goose.SetLogger(goose.NopLogger())
	if err := goose.SetDialect("sqlite3"); err != nil {
		return err
	}
	return goose.Up(db, "migrations")
}

// countMigrationFiles returns the number of embedded .sql migration files.
// Used to decide whether the schema version cache is current without querying
// the database.
func countMigrationFiles() int64 {
	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		return 0
	}
	var n int64
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".sql") {
			n++
		}
	}
	return n
}
