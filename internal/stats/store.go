package stats

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

func columns() []string {
	cols := make([]string, 0, len(Metrics)+len(Breakdowns))
	for _, m := range Metrics {
		cols = append(cols, m.Name)
	}
	return append(cols, Breakdowns...)
}

func (s *Store) Save(ctx context.Context, r Row, overwrite bool) (bool, error) {
	cols := columns()
	args := make([]any, 0, len(cols)+2)
	args = append(args, r.Day.Format(time.DateOnly))
	marks := make([]string, 0, len(cols))
	updates := make([]string, 0, len(cols)+2)
	for i, c := range cols {
		marks = append(marks, fmt.Sprintf("$%d", i+2))
		updates = append(updates, c+" = EXCLUDED."+c)
		if i < len(Metrics) {
			args = append(args, r.Values[c])
			continue
		}
		b, ok := r.Breakdown[c]
		if !ok {
			args = append(args, nil)
			continue
		}
		raw, err := json.Marshal(b)
		if err != nil {
			return false, err
		}
		args = append(args, raw)
	}
	args = append(args, r.Late)
	updates = append(updates, "late = EXCLUDED.late", "computed_at = now()")
	conflict := "DO NOTHING"
	if overwrite {
		conflict = "DO UPDATE SET " + strings.Join(updates, ", ")
	}
	sql := fmt.Sprintf("INSERT INTO daily_stats (day, %s, late) VALUES ($1::date, %s, $%d) ON CONFLICT (day) %s",
		strings.Join(cols, ", "), strings.Join(marks, ", "), len(cols)+2, conflict)
	tag, err := s.pool.Exec(ctx, sql, args...)
	if err != nil {
		return false, fmt.Errorf("save daily stats: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

func (s *Store) Range(ctx context.Context, from, to time.Time) ([]Row, error) {
	cols := columns()
	query := fmt.Sprintf("SELECT day, late, %s FROM daily_stats WHERE day BETWEEN $1::date AND $2::date ORDER BY day", strings.Join(cols, ", "))
	rows, err := s.pool.Query(ctx, query, from.Format(time.DateOnly), to.Format(time.DateOnly))
	if err != nil {
		return nil, fmt.Errorf("read daily stats: %w", err)
	}
	defer rows.Close()
	out := []Row{}
	for rows.Next() {
		var day time.Time
		var late bool
		values := make([]*int64, len(Metrics))
		raws := make([][]byte, len(Breakdowns))
		dest := []any{&day, &late}
		for i := range values {
			dest = append(dest, &values[i])
		}
		for i := range raws {
			dest = append(dest, &raws[i])
		}
		if err := rows.Scan(dest...); err != nil {
			return nil, err
		}
		r := NewRow(StartOfDay(day))
		r.Late = late
		for i, m := range Metrics {
			r.Values[m.Name] = values[i]
		}
		for i, b := range Breakdowns {
			if raws[i] == nil {
				continue
			}
			var v map[string]int64
			if err := json.Unmarshal(raws[i], &v); err != nil {
				return nil, err
			}
			r.Breakdown[b] = v
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) Latest(ctx context.Context) (time.Time, bool, error) {
	var day time.Time
	err := s.pool.QueryRow(ctx, "SELECT day FROM daily_stats ORDER BY day DESC LIMIT 1").Scan(&day)
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, err
	}
	return StartOfDay(day), true, nil
}

func (s *Store) Size(ctx context.Context) (int64, error) {
	var n int64
	err := s.pool.QueryRow(ctx, "SELECT pg_database_size(current_database())").Scan(&n)
	return n, err
}
