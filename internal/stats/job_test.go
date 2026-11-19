package stats_test

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/myronsi/messenger-moderation/internal/stats"
	"github.com/myronsi/messenger-moderation/internal/testdb"
)

func TestJobWritesEachDayOnceAndRerunOverwrites(t *testing.T) {
	m := messengerDB(t, true)
	seed(t, m)
	pool, _ := testdb.New(t)
	store := stats.NewStore(pool)
	now := at(24, 5)
	clock := func() time.Time { return now }
	job := &stats.Job{Collector: &stats.Collector{Messenger: m, Moderation: store, Usage: &usageStub{}, Now: clock}, Store: store, Now: clock}
	ctx := context.Background()

	if n, err := job.RunOnce(ctx); err != nil || n != 1 {
		t.Fatalf("first run: %d %v", n, err)
	}
	exec(t, m, `INSERT INTO users (created_at) VALUES ($1)`, at(12, 0))
	if n, err := job.RunOnce(ctx); err != nil || n != 0 {
		t.Fatalf("second run wrote %d days: %v", n, err)
	}
	rows, err := store.Range(ctx, day, day)
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows %v %v", rows, err)
	}
	want(t, rows[0], map[string]any{"users_new": 2, "users_active_day": 7, "reports_total": 3})
	if rows[0].Breakdown["reports"]["spam"] != 2 {
		t.Fatalf("stored breakdown %v", rows[0].Breakdown)
	}

	if n, err := job.Rerun(ctx, day, day); err != nil || n != 1 {
		t.Fatalf("rerun: %d %v", n, err)
	}
	rows, _ = store.Range(ctx, day, day)
	want(t, rows[0], map[string]any{"users_new": 3, "users_active_day": 7})

	now = at(24*4+1, 0)
	if n, err := job.RunOnce(ctx); err != nil || n != 3 {
		t.Fatalf("gap fill wrote %d days, want 3: %v", n, err)
	}
	rows, _ = store.Range(ctx, day, day.AddDate(0, 0, 3))
	if len(rows) != 4 || rows[1].Late != true || rows[3].Late != false {
		t.Fatalf("after the gap: %d rows, late %v %v", len(rows), rows[1].Late, rows[3].Late)
	}
	want(t, rows[1], map[string]any{"users_active_day": nil, "users_new": 1})
	if _, ok := rows[3].Get("users_active_day"); !ok {
		t.Fatal("the day that just ended has its active users")
	}
}

func row(d string, values map[string]int64, reports map[string]int64) stats.Row {
	t, _ := time.Parse(time.DateOnly, d)
	r := stats.NewRow(t)
	for k, v := range values {
		r.Set(k, v)
	}
	if reports != nil {
		r.Breakdown["reports"] = reports
	}
	return r
}

func TestCombineByWeekAndMonth(t *testing.T) {
	rows := []stats.Row{
		row("2026-11-15", map[string]int64{"messages": 10, "users_total": 100, "users_active_day": 10, "median_decision_seconds": 60}, map[string]int64{"spam": 1}),
		row("2026-11-16", map[string]int64{"messages": 20, "users_total": 101, "users_active_day": 20}, map[string]int64{"spam": 2, "scam": 1}),
		row("2026-11-17", map[string]int64{"messages": 30, "users_total": 103, "users_active_day": 25, "median_decision_seconds": 121}, nil),
		row("2026-12-01", map[string]int64{"messages": 5, "users_total": 110}, nil),
	}
	weeks := stats.Combine(rows, stats.ByWeek)
	if len(weeks) != 3 {
		t.Fatalf("%d weeks", len(weeks))
	}
	if weeks[0].Day.Format(time.DateOnly) != "2026-11-09" || weeks[1].Day.Format(time.DateOnly) != "2026-11-16" {
		t.Fatalf("weeks start on Monday: %v %v", weeks[0].Day, weeks[1].Day)
	}
	want(t, weeks[1], map[string]any{"messages": 50, "users_total": 103, "users_active_day": 23, "median_decision_seconds": 121, "calls": nil})
	if weeks[1].Breakdown["reports"]["scam"] != 1 {
		t.Fatalf("week reports %v", weeks[1].Breakdown)
	}
	months := stats.Combine(rows, stats.ByMonth)
	if len(months) != 2 || months[0].Day.Format(time.DateOnly) != "2026-11-01" {
		t.Fatalf("months %v", months)
	}
	want(t, months[0], map[string]any{"messages": 60, "users_total": 103, "median_decision_seconds": 91})
	if months[0].Breakdown["reports"]["spam"] != 3 {
		t.Fatalf("month reports %v", months[0].Breakdown)
	}
}

func TestCSV(t *testing.T) {
	rows := []stats.Row{
		row("2026-11-15", map[string]int64{"messages": 10, "reports_total": 1}, map[string]int64{"spam": 1}),
		row("2026-11-16", map[string]int64{"reports_total": 3}, map[string]int64{"scam": 3}),
	}
	var all, mod bytes.Buffer
	if err := stats.WriteCSV(&all, rows, false); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(all.String()), "\n")
	if !strings.HasPrefix(lines[0], "period,users_total,") || !strings.HasSuffix(lines[0], ",reports_scam,reports_spam,late") {
		t.Fatalf("header %q", lines[0])
	}
	if err := stats.WriteCSV(&mod, rows, true); err != nil {
		t.Fatal(err)
	}
	modLines := strings.Split(strings.TrimSpace(mod.String()), "\n")
	if modLines[0] != "period,reports_total,pack_reports_total,decisions_total,median_decision_seconds,reversals,reports_scam,reports_spam,late" {
		t.Fatalf("moderation header %q", modLines[0])
	}
	if modLines[2] != "2026-11-16,3,,,,,3,0,false" {
		t.Fatalf("moderation row %q", modLines[2])
	}
}
