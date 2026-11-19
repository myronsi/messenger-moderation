package stats

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const LateAfter = 6 * time.Hour

type Querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type Collector struct {
	Messenger  Querier
	Moderation *Store
	Usage      UsageSource
	Now        func() time.Time
}

func (c *Collector) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

func (c *Collector) Collect(ctx context.Context, day time.Time) (Row, error) {
	day = StartOfDay(day)
	now := c.now()
	end := day.AddDate(0, 0, 1)
	r := NewRow(day)
	r.Late = now.Sub(end) > LateAfter
	if end.After(now) {
		end = now
	}
	steps := []func(context.Context, Row, time.Time, time.Time) error{c.users, c.messages, c.chats, c.calls, c.reports}
	if !r.Late {
		steps = append(steps, c.snapshot)
	}
	for _, step := range steps {
		if err := step(ctx, r, day, end); err != nil {
			return Row{}, err
		}
	}
	return r, nil
}

func (c *Collector) scan(ctx context.Context, r Row, query string, names []string, args ...any) error {
	values := make([]int64, len(names))
	dest := make([]any, len(names))
	for i := range values {
		dest[i] = &values[i]
	}
	if err := c.Messenger.QueryRow(ctx, query, args...).Scan(dest...); err != nil {
		return fmt.Errorf("%s: %w", names[0], err)
	}
	for i, n := range names {
		r.Set(n, values[i])
	}
	return nil
}

func (c *Collector) users(ctx context.Context, r Row, day, end time.Time) error {
	return c.scan(ctx, r, `SELECT count(*), count(*) FILTER (WHERE created_at >= $1) FROM users WHERE created_at < $2`,
		[]string{"users_total", "users_new"}, day, end)
}

func (c *Collector) messages(ctx context.Context, r Row, day, _ time.Time) error {
	rows, err := c.Messenger.Query(ctx, `SELECT type, messages, attachments FROM message_day_counts WHERE day = $1::date`, day.Format(time.DateOnly))
	if missingTable(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("messages: %w", err)
	}
	defer rows.Close()
	var total, files int64
	byType := map[string]int64{"text": 0, "file": 0, "voice": 0, "sticker": 0}
	for rows.Next() {
		var typ string
		var n, a int64
		if err := rows.Scan(&typ, &n, &a); err != nil {
			return err
		}
		total += n
		byType[typ] += n
		if typ == "file" {
			files += a
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	r.Set("messages", total)
	r.Set("files_sent", files)
	for typ, n := range byType {
		r.Set("messages_"+typ, n)
	}
	return nil
}

func (c *Collector) chats(ctx context.Context, r Row, day, end time.Time) error {
	return c.scan(ctx, r, `SELECT
		count(*) FILTER (WHERE type = 'direct'),
		count(*) FILTER (WHERE type = 'direct' AND created_at >= $1),
		count(*) FILTER (WHERE type = 'group'),
		count(*) FILTER (WHERE type = 'group' AND created_at >= $1)
		FROM chats WHERE created_at < $2`,
		[]string{"direct_chats_total", "direct_chats_new", "groups_total", "groups_new"}, day, end)
}

func (c *Collector) calls(ctx context.Context, r Row, day, end time.Time) error {
	return c.scan(ctx, r, `SELECT count(*),
		coalesce(ceil(sum(extract(epoch FROM coalesce(ended_at, $2) - started_at)) / 60), 0)::bigint
		FROM calls WHERE started_at >= $1 AND started_at < $2`,
		[]string{"calls", "call_minutes"}, day, end)
}

func (c *Collector) reports(ctx context.Context, r Row, day, end time.Time) error {
	for _, q := range []struct{ name, query string }{
		{"reports", `SELECT reason, count(*) FROM user_reports WHERE created_at >= $1 AND created_at < $2 GROUP BY reason`},
		{"pack_reports", `SELECT reason, count(*) FROM sticker_pack_reports WHERE created_at >= $1 AND created_at < $2 GROUP BY reason`},
	} {
		byReason, total, err := c.countBy(ctx, q.query, day, end)
		if err != nil {
			return fmt.Errorf("%s: %w", q.name, err)
		}
		r.Breakdown[q.name] = byReason
		r.Set(q.name+"_total", total)
	}
	return nil
}

func (c *Collector) countBy(ctx context.Context, query string, args ...any) (map[string]int64, int64, error) {
	rows, err := c.Messenger.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := map[string]int64{}
	var total int64
	for rows.Next() {
		var key string
		var n int64
		if err := rows.Scan(&key, &n); err != nil {
			return nil, 0, err
		}
		out[key] = n
		total += n
	}
	return out, total, rows.Err()
}

func (c *Collector) snapshot(ctx context.Context, r Row, day, _ time.Time) error {
	if err := c.scan(ctx, r, `SELECT
		(SELECT count(*) FROM media_jobs WHERE state IN ('queued', 'running')),
		pg_database_size(current_database())`,
		[]string{"media_jobs_waiting", "messenger_db_bytes"}); err != nil {
		return err
	}
	if c.Moderation != nil {
		n, err := c.Moderation.Size(ctx)
		if err != nil {
			return fmt.Errorf("moderation_db_bytes: %w", err)
		}
		r.Set("moderation_db_bytes", n)
	}
	if c.Usage != nil {
		u, err := c.Usage.Usage(ctx, day)
		if err != nil {
			return err
		}
		r.Set("users_active_day", u.ActiveDay)
		r.Set("users_active_30d", u.Active30d)
		r.Set("storage_bytes", u.StorageBytes)
	}
	return nil
}

func missingTable(err error) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == "42P01"
}
