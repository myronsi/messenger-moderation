package status

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/smtp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/myronsi/messenger-moderation/internal/testdb"
)

type recorder struct {
	mu     sync.Mutex
	events []string
}

func (r *recorder) IncidentOpened(_ context.Context, in Incident) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, "open:"+in.Component)
	return nil
}

func (r *recorder) IncidentClosed(_ context.Context, in Incident) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, "close:"+in.Component+":"+in.Kind)
	return nil
}

type switchable struct {
	mu    sync.Mutex
	state map[string]State
}

func (s *switchable) set(component string, state State) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state[component] = state
}

func (s *switchable) probe(ctx context.Context) []Result {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Result
	for name, st := range s.state {
		r := Result{Component: name, State: st, Latency: 5 * time.Millisecond}
		if st == StateFailed {
			r.Detail = "connection refused"
		}
		out = append(out, r)
	}
	return out
}

func newMonitor(t *testing.T) (*Monitor, *switchable, *recorder, *time.Time) {
	t.Helper()
	pool, _ := testdb.New(t)
	sw := &switchable{state: map[string]State{"api": StateOK, "postgres": StateOK}}
	rec := &recorder{}
	m := NewMonitor(pool, Options{Probes: []Probe{sw.probe}, Notifier: rec, Interval: time.Minute})
	now := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	m.now = func() time.Time { return now }
	return m, sw, rec, &now
}

func tick(t *testing.T, m *Monitor, now *time.Time) {
	t.Helper()
	*now = now.Add(time.Minute)
	if err := m.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestIncidentOpensAfterThreeFailuresAndCloses(t *testing.T) {
	m, sw, rec, now := newMonitor(t)
	ctx := context.Background()
	tick(t, m, now)
	sw.set("postgres", StateFailed)
	tick(t, m, now)
	tick(t, m, now)
	if len(rec.events) != 0 {
		t.Fatalf("opened too early: %v", rec.events)
	}
	tick(t, m, now)
	if strings.Join(rec.events, ",") != "open:postgres" {
		t.Fatalf("events = %v", rec.events)
	}
	tick(t, m, now)
	if len(rec.events) != 1 {
		t.Fatalf("must not open twice: %v", rec.events)
	}

	reporter := NewReporter(m.pool)
	o, err := reporter.Overview(ctx, *now)
	if err != nil {
		t.Fatal(err)
	}
	if len(o.Open) != 1 || !o.Open[0].StartedAt.Equal(time.Date(2026, 11, 19, 12, 2, 0, 0, time.UTC)) {
		t.Fatalf("open = %+v", o.Open)
	}
	p, err := reporter.Public(ctx, *now)
	if err != nil {
		t.Fatal(err)
	}
	if p.State != "down" || len(p.Areas) != 1 || p.Areas[0].Name != "messages" || p.Areas[0].State != "down" || p.Areas[0].Since == nil {
		t.Fatalf("public = %+v", p)
	}

	sw.set("postgres", StateOK)
	tick(t, m, now)
	if strings.Join(rec.events, ",") != "open:postgres,close:postgres:failure" {
		t.Fatalf("events = %v", rec.events)
	}
	if p, _ := reporter.Public(ctx, *now); p.State != "working" {
		t.Fatalf("public after recovery = %+v", p)
	}
}

func TestSlowChecks(t *testing.T) {
	m, _, _, now := newMonitor(t)
	m.probes = []Probe{func(context.Context) []Result {
		return []Result{{Component: "elasticsearch", State: StateOK, Latency: 3 * time.Second}}
	}}
	tick(t, m, now)
	p, err := NewReporter(m.pool).Public(context.Background(), *now)
	if err != nil {
		t.Fatal(err)
	}
	if p.State != "slow" || p.Areas[0].Name != "search" || p.Areas[0].State != "slow" {
		t.Fatalf("public = %+v", p)
	}
}

func TestMonitoringGapIsRecorded(t *testing.T) {
	m, _, rec, now := newMonitor(t)
	tick(t, m, now)
	*now = now.Add(2 * time.Hour)
	if err := m.RecordGap(context.Background()); err != nil {
		t.Fatal(err)
	}
	if strings.Join(rec.events, ",") != "close:monitoring:monitoring_gap" {
		t.Fatalf("events = %v", rec.events)
	}
	if err := m.RecordGap(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestRollupKeepsDailyTotalsAndPurgesOldChecks(t *testing.T) {
	m, sw, _, now := newMonitor(t)
	ctx := context.Background()
	start := *now
	*now = start.Add(-40 * 24 * time.Hour)
	tick(t, m, now)
	*now = start.Add(-24 * time.Hour)
	sw.set("api", StateFailed)
	tick(t, m, now)
	*now = start
	if err := m.Rollup(ctx); err != nil {
		t.Fatal(err)
	}
	var raw int
	if err := m.pool.QueryRow(ctx, `SELECT count(*) FROM service_checks`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if raw != 2 {
		t.Fatalf("raw checks left = %d, want the two from yesterday", raw)
	}
	var failed int
	day := start.Add(-24 * time.Hour).UTC().Format(time.DateOnly)
	if err := m.pool.QueryRow(ctx, `SELECT failed FROM service_uptime_daily WHERE component = 'api' AND day = $1`, day).Scan(&failed); err != nil || failed != 1 {
		t.Fatalf("daily failed = %d, %v", failed, err)
	}
	o, err := NewReporter(m.pool).Overview(ctx, *now)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range o.Components {
		if c.Component == "api" && (c.Uptime30d == nil || *c.Uptime30d != 0) {
			t.Fatalf("api uptime = %v", c.Uptime30d)
		}
	}
}

func TestReadinessProbe(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"status":"not_ready","checks":{"postgres":"ok","scylla":"unavailable"}}`))
	}))
	defer srv.Close()
	got := map[string]State{}
	for _, r := range ReadinessProbe(srv.Client(), srv.URL)(context.Background()) {
		got[r.Component] = r.State
	}
	if got["api"] != StateOK || got["postgres"] != StateOK || got["scylla"] != StateFailed {
		t.Fatalf("results = %v", got)
	}
	down := ReadinessProbe(srv.Client(), "http://127.0.0.1:1")(context.Background())
	if len(down) != 1 || down[0].Component != "api" || down[0].State != StateFailed {
		t.Fatalf("unreachable = %+v", down)
	}
}

func TestMailNotifier(t *testing.T) {
	var sent string
	n := NewMailNotifier(MailConfig{Host: "smtp.example.com", Port: 587, From: "status@example.com", To: "ops@example.com", Label: "staging"})
	n.send = func(addr string, _ smtp.Auth, from string, to []string, msg []byte) error {
		if addr != "smtp.example.com:587" || from != "status@example.com" || to[0] != "ops@example.com" {
			return errors.New("wrong envelope")
		}
		sent = string(msg)
		return nil
	}
	in := Incident{Component: "scylla", StartedAt: time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC), Detail: "timeout"}
	if err := n.IncidentOpened(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sent, "Subject: [staging] scylla is down") || !strings.Contains(sent, "timeout") {
		t.Fatalf("mail = %q", sent)
	}
}
