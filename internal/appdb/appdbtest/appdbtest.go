package appdbtest

import (
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"net/url"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/myronsi/messenger-moderation/internal/appdb"
)

//go:embed messenger_schema.sql
var schemaSQL string

type DB struct {
	Read  *pgxpool.Pool
	Write *pgxpool.Pool
}

func New(t *testing.T) DB {
	t.Helper()
	base := os.Getenv("TEST_DATABASE_URL")
	if base == "" {
		t.Skip("TEST_DATABASE_URL is not set; start the test databases with messenger-back's scripts/test-deps.sh")
	}
	ctx := context.Background()
	suffix := make([]byte, 6)
	if _, err := rand.Read(suffix); err != nil {
		t.Fatal(err)
	}
	schema := "ma_" + hex.EncodeToString(suffix)

	admin, err := pgx.Connect(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
		_ = admin.Close(context.Background())
	})
	if _, err := admin.Exec(ctx, "SET search_path TO "+schema+"; "+schemaSQL, pgx.QueryExecModeSimpleProtocol); err != nil {
		t.Fatal(err)
	}

	u, err := url.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()

	write, err := pgxpool.New(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(write.Close)
	read, err := appdb.Open(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(read.Close)
	return DB{Read: read, Write: write}
}
