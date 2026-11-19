package stats_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/myronsi/messenger-moderation/internal/stats"
)

func TestDeployGuideListsTheReadGrants(t *testing.T) {
	guide, err := os.ReadFile("../../docs/deploy.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, g := range stats.ReadGrants {
		if !strings.Contains(string(guide), "`"+g+"`") {
			t.Errorf("docs/deploy.md does not list %q", g)
		}
	}
}

func TestCollectNeedsOnlyTheReadGrants(t *testing.T) {
	m := messengerDB(t, true)
	seed(t, m)
	ctx := context.Background()
	schema := m.Config().ConnConfig.RuntimeParams["search_path"]
	suffix := make([]byte, 6)
	if _, err := rand.Read(suffix); err != nil {
		t.Fatal(err)
	}
	role := "reader_" + hex.EncodeToString(suffix)
	exec(t, m, "CREATE ROLE "+role+" NOLOGIN")
	t.Cleanup(func() {
		_, _ = m.Exec(context.Background(), "DROP OWNED BY "+role)
		_, _ = m.Exec(context.Background(), "DROP ROLE "+role)
	})
	exec(t, m, "GRANT USAGE ON SCHEMA "+schema+" TO "+role)
	for _, g := range stats.ReadGrants {
		exec(t, m, strings.ReplaceAll(g, "moderation_reader", role))
	}

	cfg := m.Config().Copy()
	cfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		_, err := conn.Exec(ctx, "SET ROLE "+role)
		return err
	}
	reader, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	exec(t, m, "ALTER TABLE users ADD COLUMN password_hash TEXT")
	if _, err := reader.Exec(ctx, "SELECT password_hash FROM users"); err == nil {
		t.Fatal("the reader can read a column it was not granted")
	}

	c := &stats.Collector{Messenger: reader, Now: func() time.Time { return at(24, 5) }}
	r, err := c.Collect(ctx, day)
	if err != nil {
		t.Fatalf("collect as the reader: %v", err)
	}
	want(t, r, map[string]any{"users_total": 4, "users_new": 2, "messages": 0, "calls": 3, "media_jobs_waiting": 2, "reports_total": 3, "pack_reports_total": 1})
}
