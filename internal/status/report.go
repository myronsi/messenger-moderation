package status

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type ComponentState struct {
	Component string    `json:"component"`
	State     State     `json:"state"`
	LatencyMS int64     `json:"latency_ms"`
	Detail    string    `json:"detail,omitempty"`
	CheckedAt time.Time `json:"checked_at"`
	Uptime30d *float64  `json:"uptime_30d,omitempty"`
}

type Overview struct {
	Components []ComponentState `json:"components"`
	Open       []Incident       `json:"open_incidents"`
	Recent     []Incident       `json:"recent_incidents"`
}

type Area struct {
	Name  string     `json:"name"`
	State string     `json:"state"`
	Since *time.Time `json:"since,omitempty"`
}

type Public struct {
	State     string    `json:"state"`
	Areas     []Area    `json:"areas"`
	UpdatedAt time.Time `json:"updated_at"`
}

var publicAreas = []struct {
	name       string
	components []string
}{
	{"messages", []string{"api", "postgres", "redis", "scylla", "ids", "worker"}},
	{"files", []string{"s3", "object_storage", "media"}},
	{"search", []string{"elasticsearch", "search"}},
	{"calls", []string{"livekit"}},
}

type Reporter struct{ pool *pgxpool.Pool }

func NewReporter(pool *pgxpool.Pool) *Reporter { return &Reporter{pool: pool} }

func (r *Reporter) Overview(ctx context.Context, now time.Time) (Overview, error) {
	o := Overview{Components: []ComponentState{}, Open: []Incident{}, Recent: []Incident{}}
	rows, err := r.pool.Query(ctx, `SELECT DISTINCT ON (component) component, status, latency_ms, coalesce(detail, ''), checked_at
		FROM service_checks ORDER BY component, checked_at DESC, id DESC`)
	if err != nil {
		return o, err
	}
	for rows.Next() {
		var c ComponentState
		if err := rows.Scan(&c.Component, &c.State, &c.LatencyMS, &c.Detail, &c.CheckedAt); err != nil {
			rows.Close()
			return o, err
		}
		o.Components = append(o.Components, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return o, err
	}
	uptime, err := r.uptime(ctx, now)
	if err != nil {
		return o, err
	}
	for i := range o.Components {
		if u, ok := uptime[o.Components[i].Component]; ok {
			o.Components[i].Uptime30d = &u
		}
	}
	if o.Open, err = r.incidents(ctx, `WHERE ended_at IS NULL ORDER BY started_at DESC`); err != nil {
		return o, err
	}
	o.Recent, err = r.incidents(ctx, `ORDER BY started_at DESC LIMIT 50`)
	return o, err
}

func (r *Reporter) Public(ctx context.Context, now time.Time) (Public, error) {
	o, err := r.Overview(ctx, now)
	if err != nil {
		return Public{}, err
	}
	latest := map[string]ComponentState{}
	for _, c := range o.Components {
		latest[c.Component] = c
	}
	open := map[string]time.Time{}
	for _, in := range o.Open {
		open[in.Component] = in.StartedAt
	}
	p := Public{State: "working", Areas: []Area{}, UpdatedAt: now}
	for _, area := range publicAreas {
		a := Area{Name: area.name, State: "unknown"}
		for _, name := range area.components {
			c, ok := latest[name]
			if !ok {
				continue
			}
			if a.State == "unknown" {
				a.State = "working"
			}
			switch {
			case c.State == StateFailed:
				a.State = "down"
				if since, ok := open[name]; ok && (a.Since == nil || since.Before(*a.Since)) {
					a.Since = &since
				}
			case c.State == StateSlow && a.State == "working":
				a.State = "slow"
			}
		}
		if a.State == "unknown" {
			continue
		}
		p.Areas = append(p.Areas, a)
		if a.State == "down" {
			p.State = "down"
		} else if a.State == "slow" && p.State == "working" {
			p.State = "slow"
		}
	}
	return p, nil
}

func (r *Reporter) uptime(ctx context.Context, now time.Time) (map[string]float64, error) {
	since := now.UTC().Truncate(24 * time.Hour).Add(-29 * 24 * time.Hour)
	rows, err := r.pool.Query(ctx, `
		WITH counts AS (
			SELECT component, ok + slow AS good, ok + slow + failed AS total FROM service_uptime_daily WHERE day >= $1::date
			UNION ALL
			SELECT component, count(*) FILTER (WHERE status <> 'failed'), count(*) FROM service_checks
			WHERE checked_at >= $2 GROUP BY component
		)
		SELECT component, sum(good)::float8 / NULLIF(sum(total), 0)::float8 FROM counts GROUP BY component`, since, now.UTC().Truncate(24*time.Hour))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]float64{}
	for rows.Next() {
		var name string
		var ratio *float64
		if err := rows.Scan(&name, &ratio); err != nil {
			return nil, err
		}
		if ratio != nil {
			out[name] = *ratio
		}
	}
	return out, rows.Err()
}

func (r *Reporter) incidents(ctx context.Context, tail string) ([]Incident, error) {
	rows, err := r.pool.Query(ctx, `SELECT id, component, kind, started_at, ended_at, coalesce(detail, '') FROM incidents `+tail)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []Incident{}
	for rows.Next() {
		var in Incident
		if err := rows.Scan(&in.ID, &in.Component, &in.Kind, &in.StartedAt, &in.EndedAt, &in.Detail); err != nil {
			return nil, err
		}
		list = append(list, in)
	}
	return list, rows.Err()
}
