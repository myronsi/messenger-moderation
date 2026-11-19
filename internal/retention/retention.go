package retention

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

const RunEvery = 24 * time.Hour

type Execer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

type Rule struct {
	Name   string
	Table  string
	Column string
	Keep   string
}

var Rules = []Rule{
	{Name: "audit log", Table: "audit_log", Column: "created_at", Keep: "2 years"},
	{Name: "incidents", Table: "incidents", Column: "ended_at", Keep: "2 years"},
}

const batch = 5000

func Purge(ctx context.Context, db Execer, rules []Rule) (map[string]int64, error) {
	out := map[string]int64{}
	for _, r := range rules {
		query := fmt.Sprintf(`DELETE FROM %[1]s WHERE ctid IN (SELECT ctid FROM %[1]s WHERE %[2]s < now() - interval '%[3]s' LIMIT %[4]d)`,
			r.Table, r.Column, r.Keep, batch)
		for {
			tag, err := db.Exec(ctx, query)
			if err != nil {
				return out, fmt.Errorf("purge %s: %w", r.Name, err)
			}
			out[r.Name] += tag.RowsAffected()
			if tag.RowsAffected() < batch {
				break
			}
		}
	}
	return out, nil
}

func Run(ctx context.Context, db Execer, log *slog.Logger, every time.Duration) {
	run := func() {
		purged, err := Purge(ctx, db, Rules)
		if err != nil && ctx.Err() == nil {
			log.ErrorContext(ctx, "retention failed", "error", err)
		}
		for name, n := range purged {
			if n > 0 {
				log.InfoContext(ctx, "retention purged rows", "data", name, "rows", n)
			}
		}
	}
	run()
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			run()
		}
	}
}
