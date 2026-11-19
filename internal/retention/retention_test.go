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

func TestDecidedCasesAndContentDecisionsAreKeptForTwoYears(t *testing.T) {
	pool, _ := testdb.New(t)
	ctx := context.Background()
	now := time.Now()
	old := now.AddDate(-3, 0, 0)
	var staffID int64
	if err := pool.QueryRow(ctx, `INSERT INTO staff_accounts (username, password_hash, totp_secret, role)
		VALUES ('retention_admin', 'x', '\x00', 'admin') RETURNING id`).Scan(&staffID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO cases (kind, source_key, user_id, to_status, opened_at, deadline, state, decided_at, decided_by) VALUES
		('account_status', 'old-decided', 1, 'restricted', $1, $1, 'confirmed', $1, $3),
		('account_status', 'recent-decided', 2, 'restricted', $2, $2, 'confirmed', $2, $3),
		('account_status', 'old-open', 3, 'suspended', $1, $1, 'open', NULL, NULL),
		('notice', 'old-notice', NULL, NULL, $1, $1, 'confirmed', $1, $3)`, old, now, staffID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO notices (case_id, notifier_name, content, location, explanation, received_at, entered_by)
		SELECT id, 'Notifier', 'content', 'location', 'explanation', $1, $2 FROM cases WHERE source_key = 'old-notice'`, old, staffID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO content_decisions (chat_id, message_id, action, staff_id, decided_at) VALUES
		(10, 1, 'removed', $3, $1), (10, 2, 'no_violation', $3, $2)`, old, now, staffID); err != nil {
		t.Fatal(err)
	}
	purged, err := retention.Purge(ctx, pool, retention.Rules)
	if err != nil {
		t.Fatal(err)
	}
	if purged["cases"] != 2 || purged["content decisions"] != 1 {
		t.Fatalf("purged %v, want the 2 cases and the 1 content decision decided 3 years ago", purged)
	}
	var cases, notices, decisions int
	if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM cases), (SELECT count(*) FROM notices), (SELECT count(*) FROM content_decisions)`).
		Scan(&cases, &notices, &decisions); err != nil {
		t.Fatal(err)
	}
	if cases != 2 || notices != 0 || decisions != 1 {
		t.Fatalf("left cases=%d notices=%d decisions=%d: a recent or still open case stays, a deleted case takes its notice", cases, notices, decisions)
	}
}
