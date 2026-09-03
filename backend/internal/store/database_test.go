package store

import (
	"context"
	"testing"
)

func TestSQLiteDatabaseBeginRollbackAfterCallerFailure(t *testing.T) {
	db, err := OpenSQLite(t.TempDir() + "/database.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	if _, err := db.ExecContext(ctx, "CREATE TABLE records (id TEXT NOT NULL)"); err != nil {
		t.Fatal(err)
	}
	database, err := databaseFrom(db)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := database.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, "INSERT INTO records (id) VALUES ($1)", "failed"); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, "INSERT INTO missing (id) VALUES ($1)", "error"); err == nil {
		t.Fatal("invalid transaction statement succeeded")
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}

	var count int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM records").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("rolled-back transaction left %d records", count)
	}
}
