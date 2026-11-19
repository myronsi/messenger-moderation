package status

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	failuresToOpen = 3
	rawRetention   = 30 * 24 * time.Hour
	monitoringName = "monitoring"
)

type Incident struct {
	ID        int64      `json:"id"`
	Component string     `json:"component"`
	Kind      string     `json:"kind"`
	StartedAt time.Time  `json:"started_at"`
	EndedAt   *time.Time `json:"ended_at,omitempty"`
	Detail    string     `json:"detail,omitempty"`
}

type Notifier interface {
	IncidentOpened(ctx context.Context, in Incident) error
	IncidentClosed(ctx context.Context, in Incident) error
}

type Monitor struct {
	pool      *pgxpool.Pool
	probes    []Probe
	notifier  Notifier
	log       *slog.Logger
	interval  time.Duration
	timeout   time.Duration
	slowAfter time.Duration
	now       func() time.Time
}

type Options struct {
	Probes    []Probe
	Notifier  Notifier
	Log       *slog.Logger
	Interval  time.Duration
	Timeout   time.Duration
	SlowAfter time.Duration
}

func NewMonitor(pool *pgxpool.Pool, o Options) *Monitor {
	m := &Monitor{pool: pool, probes: o.Probes, notifier: o.Notifier, log: o.Log, interval: o.Interval,
		timeout: o.Timeout, slowAfter: o.SlowAfter, now: time.Now}
	if m.log == nil {
		m.log = slog.Default()
	}
	if m.interval <= 0 {
		m.interval = time.Minute
	}
	if m.timeout <= 0 {
		m.timeout = 10 * time.Second
	}
	if m.slowAfter <= 0 {
		m.slowAfter = time.Second
	}
	return m
}

func (m *Monitor) Run(ctx context.Context) {
	if err := m.RecordGap(ctx); err != nil {
		m.log.WarnContext(ctx, "status: record monitoring gap", "error", err)
	}
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()
	lastDaily := time.Time{}
	for {
		if err := m.RunOnce(ctx); err != nil && !errors.Is(err, context.Canceled) {
			m.log.WarnContext(ctx, "status: run checks", "error", err)
		}
		if m.now().Sub(lastDaily) >= time.Hour {
			if err := m.Rollup(ctx); err != nil && !errors.Is(err, context.Canceled) {
				m.log.WarnContext(ctx, "status: roll up checks", "error", err)
			}
			lastDaily = m.now()
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (m *Monitor) RunOnce(ctx context.Context) error {
	checkedAt := m.now()
	probeCtx, cancel := context.WithTimeout(ctx, m.timeout)
	defer cancel()

	var (
		mu      sync.Mutex
		wg      sync.WaitGroup
		results []Result
	)
	for _, probe := range m.probes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got := probe(probeCtx)
			mu.Lock()
			results = append(results, got...)
			mu.Unlock()
		}()
	}
	wg.Wait()

	for _, r := range results {
		if r.State == StateOK && r.Latency > m.slowAfter {
			r.State = StateSlow
		}
		if err := m.record(ctx, r, checkedAt); err != nil {
			return err
		}
	}
	return nil
}

func (m *Monitor) record(ctx context.Context, r Result, at time.Time) error {
	var opened, closed *Incident
	err := pgx.BeginFunc(ctx, m.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO service_checks (component, status, latency_ms, detail, checked_at) VALUES ($1, $2, $3, NULLIF($4, ''), $5)`,
			r.Component, string(r.State), r.Latency.Milliseconds(), r.Detail, at); err != nil {
			return err
		}
		if r.State == StateFailed {
			in, err := openIfFailing(ctx, tx, r)
			opened = in
			return err
		}
		in, err := closeOpen(ctx, tx, r.Component, at)
		closed = in
		return err
	})
	if err != nil {
		return err
	}
	if opened != nil {
		m.notify(ctx, *opened, true)
	}
	if closed != nil {
		m.notify(ctx, *closed, false)
	}
	return nil
}

func openIfFailing(ctx context.Context, tx pgx.Tx, r Result) (*Incident, error) {
	var open bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM incidents WHERE component = $1 AND ended_at IS NULL)`, r.Component).Scan(&open); err != nil || open {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT status, checked_at FROM service_checks WHERE component = $1 ORDER BY checked_at DESC, id DESC LIMIT $2`,
		r.Component, failuresToOpen)
	if err != nil {
		return nil, err
	}
	var (
		failed int
		first  time.Time
	)
	for rows.Next() {
		var s string
		var at time.Time
		if err := rows.Scan(&s, &at); err != nil {
			rows.Close()
			return nil, err
		}
		if s == string(StateFailed) {
			failed++
			first = at
		}
	}
	rows.Close()
	if failed < failuresToOpen {
		return nil, rows.Err()
	}
	in := Incident{Component: r.Component, Kind: "failure", StartedAt: first, Detail: r.Detail}
	err = tx.QueryRow(ctx, `INSERT INTO incidents (component, kind, started_at, detail) VALUES ($1, 'failure', $2, NULLIF($3, '')) RETURNING id`,
		in.Component, in.StartedAt, in.Detail).Scan(&in.ID)
	return &in, err
}

func closeOpen(ctx context.Context, tx pgx.Tx, component string, at time.Time) (*Incident, error) {
	var in Incident
	var detail *string
	err := tx.QueryRow(ctx, `UPDATE incidents SET ended_at = $2 WHERE component = $1 AND ended_at IS NULL
		RETURNING id, component, kind, started_at, ended_at, detail`, component, at).
		Scan(&in.ID, &in.Component, &in.Kind, &in.StartedAt, &in.EndedAt, &detail)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if detail != nil {
		in.Detail = *detail
	}
	return &in, err
}

func (m *Monitor) RecordGap(ctx context.Context) error {
	var last *time.Time
	if err := m.pool.QueryRow(ctx, `SELECT max(checked_at) FROM service_checks`).Scan(&last); err != nil || last == nil {
		return err
	}
	now := m.now()
	if now.Sub(*last) <= failuresToOpen*m.interval {
		return nil
	}
	in := Incident{Component: monitoringName, Kind: "monitoring_gap", StartedAt: *last, EndedAt: &now,
		Detail: fmt.Sprintf("no checks for %s: the server or the moderation service was not running", now.Sub(*last).Round(time.Minute))}
	if err := m.pool.QueryRow(ctx, `INSERT INTO incidents (component, kind, started_at, ended_at, detail) VALUES ($1, 'monitoring_gap', $2, $3, $4) RETURNING id`,
		in.Component, in.StartedAt, in.EndedAt, in.Detail).Scan(&in.ID); err != nil {
		return err
	}
	m.notify(ctx, in, false)
	return nil
}

func (m *Monitor) Rollup(ctx context.Context) error {
	today := m.now().UTC().Truncate(24 * time.Hour)
	return pgx.BeginFunc(ctx, m.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO service_uptime_daily (component, day, ok, slow, failed)
			SELECT component, (checked_at AT TIME ZONE 'UTC')::date,
				count(*) FILTER (WHERE status = 'ok'), count(*) FILTER (WHERE status = 'slow'), count(*) FILTER (WHERE status = 'failed')
			FROM service_checks WHERE checked_at >= $1 AND checked_at < $2
			GROUP BY 1, 2
			ON CONFLICT (component, day) DO UPDATE SET ok = EXCLUDED.ok, slow = EXCLUDED.slow, failed = EXCLUDED.failed`,
			today.Add(-rawRetention), today); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `DELETE FROM service_checks WHERE checked_at < $1`, today.Add(-rawRetention))
		return err
	})
}

func (m *Monitor) notify(ctx context.Context, in Incident, opened bool) {
	if m.notifier == nil {
		return
	}
	var err error
	if opened {
		err = m.notifier.IncidentOpened(ctx, in)
	} else {
		err = m.notifier.IncidentClosed(ctx, in)
	}
	if err != nil {
		m.log.WarnContext(ctx, "status: send alert", "component", in.Component, "error", err)
	}
}
