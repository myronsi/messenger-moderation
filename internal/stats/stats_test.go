package stats_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/myronsi/messenger-moderation/internal/stats"
	"github.com/myronsi/messenger-moderation/internal/testdb"
)

const messengerSchema = `
CREATE TABLE users (id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY, created_at TIMESTAMPTZ NOT NULL);
CREATE TABLE user_sessions (user_id BIGINT NOT NULL, last_active_at TIMESTAMPTZ NOT NULL);
CREATE TABLE chats (id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY, type TEXT NOT NULL, created_at TIMESTAMPTZ NOT NULL);
CREATE TABLE calls (id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY, started_at TIMESTAMPTZ, ended_at TIMESTAMPTZ);
CREATE TABLE attachments (size BIGINT NOT NULL);
CREATE TABLE media_jobs (state TEXT NOT NULL);
CREATE TABLE user_reports (reason TEXT NOT NULL, created_at TIMESTAMPTZ NOT NULL);
CREATE TABLE sticker_pack_reports (reason TEXT NOT NULL, created_at TIMESTAMPTZ NOT NULL);
`

const dayCounts = `CREATE TABLE message_day_counts (day DATE NOT NULL, type TEXT NOT NULL, messages BIGINT NOT NULL, attachments BIGINT NOT NULL, PRIMARY KEY (day, type));`

func messengerDB(t *testing.T, withCounts bool) *pgxpool.Pool {
	t.Helper()
	base := os.Getenv("TEST_DATABASE_URL")
	if base == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	suffix := make([]byte, 6)
	if _, err := rand.Read(suffix); err != nil {
		t.Fatal(err)
	}
	schema := "ms_" + hex.EncodeToString(suffix)
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
	u, err := url.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	pool, err := pgxpool.New(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	ddl := messengerSchema
	if withCounts {
		ddl += dayCounts
	}
	if _, err := pool.Exec(ctx, ddl); err != nil {
		t.Fatal(err)
	}
	return pool
}

func exec(t *testing.T, db *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	if _, err := db.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
}

var day = time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC)

func at(h, m int) time.Time {
	return day.Add(time.Duration(h)*time.Hour + time.Duration(m)*time.Minute)
}

func seed(t *testing.T, db *pgxpool.Pool) {
	exec(t, db, `INSERT INTO users (created_at) VALUES ($1), ($2), ($3), ($4), ($5)`, at(-48, 0), at(-30, 0), at(9, 0), at(23, 59), at(24, 1))
	exec(t, db, `INSERT INTO user_sessions (user_id, last_active_at) VALUES (1, $1), (1, $2), (2, $3), (3, $4), (4, $5)`,
		at(10, 0), at(24, 2), at(-1, 0), at(-24*20, 0), at(-24*40, 0))
	exec(t, db, `INSERT INTO chats (type, created_at) VALUES ('direct', $1), ('direct', $2), ('group', $3), ('group', $4), ('group', $5)`,
		at(-30, 0), at(12, 0), at(-30, 0), at(13, 0), at(24, 30))
	exec(t, db, `INSERT INTO calls (started_at, ended_at) VALUES ($1, $2), ($3, $4), ($5, NULL), ($6, NULL)`,
		at(10, 0), at(10, 5), at(23, 50), at(24, 20), at(23, 59), at(-2, 0))
	exec(t, db, `INSERT INTO attachments (size) VALUES (1000), (2500)`)
	exec(t, db, `INSERT INTO media_jobs (state) VALUES ('queued'), ('running'), ('done'), ('failed')`)
	exec(t, db, `INSERT INTO user_reports (reason, created_at) VALUES ('spam', $1), ('spam', $1), ('scam', $2), ('spam', $3)`, at(1, 0), at(20, 0), at(24, 0))
	exec(t, db, `INSERT INTO sticker_pack_reports (reason, created_at) VALUES ('hate', $1)`, at(5, 0))
}

func want(t *testing.T, r stats.Row, expected map[string]any) {
	t.Helper()
	for name, w := range expected {
		v, ok := r.Get(name)
		switch w := w.(type) {
		case nil:
			if ok {
				t.Errorf("%s = %d, want empty", name, v)
			}
		case int:
			if !ok || v != int64(w) {
				t.Errorf("%s = %d (set %v), want %d", name, v, ok, w)
			}
		}
	}
}

func TestCollectRightAfterMidnight(t *testing.T) {
	m := messengerDB(t, true)
	seed(t, m)
	exec(t, m, `INSERT INTO message_day_counts VALUES ($1::date, 'text', 40, 0), ($1::date, 'file', 5, 9), ($1::date, 'voice', 3, 3), ('2026-11-19', 'text', 99, 0)`, "2026-11-18")
	pool, _ := testdb.New(t)
	c := &stats.Collector{Messenger: m, Moderation: stats.NewStore(pool), Now: func() time.Time { return at(24, 5) }}
	r, err := c.Collect(context.Background(), day)
	if err != nil {
		t.Fatal(err)
	}
	if r.Late {
		t.Fatal("a day computed 5 minutes after it ended is not late")
	}
	want(t, r, map[string]any{
		"users_total": 4, "users_new": 2, "users_active_day": 1, "users_active_30d": 3,
		"messages": 48, "messages_text": 40, "messages_file": 5, "messages_voice": 3, "messages_sticker": 0, "files_sent": 9,
		"direct_chats_total": 2, "direct_chats_new": 1, "groups_total": 2, "groups_new": 1,
		"calls": 3, "call_minutes": 36,
		"files_bytes": 3500, "media_jobs_waiting": 2, "storage_bytes": nil,
		"reports_total": 3, "pack_reports_total": 1, "decisions_total": nil, "median_decision_seconds": nil, "reversals": nil,
	})
	if v, ok := r.Get("messenger_db_bytes"); !ok || v <= 0 {
		t.Errorf("messenger_db_bytes = %d", v)
	}
	if v, ok := r.Get("moderation_db_bytes"); !ok || v <= 0 {
		t.Errorf("moderation_db_bytes = %d", v)
	}
	if got := r.Breakdown["reports"]; got["spam"] != 2 || got["scam"] != 1 || len(got) != 2 {
		t.Errorf("reports = %v", got)
	}
	if got := r.Breakdown["pack_reports"]; got["hate"] != 1 {
		t.Errorf("pack_reports = %v", got)
	}
}

func TestCollectLateLeavesPointInTimeNumbersEmpty(t *testing.T) {
	m := messengerDB(t, false)
	seed(t, m)
	c := &stats.Collector{Messenger: m, Now: func() time.Time { return at(24*3, 0) }}
	r, err := c.Collect(context.Background(), day)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Late {
		t.Fatal("a day computed two days after it ended is late")
	}
	want(t, r, map[string]any{
		"users_total": 4, "users_new": 2, "groups_new": 1, "reports_total": 3,
		"users_active_day": nil, "users_active_30d": nil, "files_bytes": nil, "media_jobs_waiting": nil, "messenger_db_bytes": nil,
		"messages": nil, "files_sent": nil,
	})
}

func TestCollectTodaySoFar(t *testing.T) {
	m := messengerDB(t, true)
	seed(t, m)
	c := &stats.Collector{Messenger: m, Now: func() time.Time { return at(11, 0) }}
	r, err := c.Collect(context.Background(), day)
	if err != nil {
		t.Fatal(err)
	}
	want(t, r, map[string]any{"users_new": 1, "direct_chats_new": 0, "calls": 1, "call_minutes": 5, "reports_total": 2, "messages": 0})
}
