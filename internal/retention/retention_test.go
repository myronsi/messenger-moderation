package retention_test

import (
	"context"
	"testing"
	"time"

	"github.com/myronsi/messenger-moderation/internal/retention"
	"github.com/myronsi/messenger-moderation/internal/testdb"
)

func TestAuditLogIsKeptForTwoYears(t *testing.T) {
	pool, _ := testdb.New(t)
	ctx := context.Background()
	now := time.Now()
	for _, age := range []time.Duration{0, 365 * 24 * time.Hour, 729 * 24 * time.Hour, 731 * 24 * time.Hour, 900 * 24 * time.Hour} {
		if _, err := pool.Exec(ctx, `INSERT INTO audit_log (action, created_at) VALUES ('test', $1)`, now.Add(-age)); err != nil {
			t.Fatal(err)
		}
	}
	purged, err := retention.Purge(ctx, pool, retention.Rules)
	if err != nil {
		t.Fatal(err)
	}
	if purged["audit log"] != 2 {
		t.Fatalf("purged %v, want the 2 rows older than 2 years", purged)
	}
	var left int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_log`).Scan(&left); err != nil || left != 3 {
		t.Fatalf("left %d %v", left, err)
	}
	if purged, err := retention.Purge(ctx, pool, retention.Rules); err != nil || purged["audit log"] != 0 {
		t.Fatalf("second purge %v %v", purged, err)
	}
}

func TestClosedIncidentsAreKeptForTwoYears(t *testing.T) {
	pool, _ := testdb.New(t)
	ctx := context.Background()
	old := time.Now().AddDate(-3, 0, 0)
	if _, err := pool.Exec(ctx, `INSERT INTO incidents (component, started_at, ended_at) VALUES
		('api', $1, $2), ('redis', $1, $3), ('worker', $1, NULL)`, old, old.Add(time.Hour), time.Now()); err != nil {
		t.Fatal(err)
	}
	purged, err := retention.Purge(ctx, pool, retention.Rules)
	if err != nil || purged["incidents"] != 1 {
		t.Fatalf("purged %v %v, want the incident that ended 3 years ago", purged, err)
	}
	var left []string
	rows, err := pool.Query(ctx, `SELECT component FROM incidents ORDER BY component`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			t.Fatal(err)
		}
		left = append(left, c)
	}
	if len(left) != 2 || left[0] != "redis" || left[1] != "worker" {
		t.Fatalf("left %v: an incident that ended recently or is still open stays", left)
	}
}
