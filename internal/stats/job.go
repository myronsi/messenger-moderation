package stats

import (
	"context"
	"log/slog"
	"time"
)

const (
	RunEvery    = 15 * time.Minute
	MaxBackfill = 31
)

type Job struct {
	Collector *Collector
	Store     *Store
	Log       *slog.Logger
	Now       func() time.Time
}

func (j *Job) now() time.Time {
	if j.Now != nil {
		return j.Now()
	}
	return time.Now()
}

func (j *Job) RunOnce(ctx context.Context) (int, error) {
	yesterday := StartOfDay(j.now()).AddDate(0, 0, -1)
	from := yesterday
	latest, ok, err := j.Store.Latest(ctx)
	if err != nil {
		return 0, err
	}
	if ok {
		if !latest.Before(yesterday) {
			return 0, nil
		}
		from = latest.AddDate(0, 0, 1)
		if oldest := yesterday.AddDate(0, 0, 1-MaxBackfill); from.Before(oldest) {
			from = oldest
		}
	}
	return j.write(ctx, from, yesterday, false)
}

func (j *Job) Rerun(ctx context.Context, from, to time.Time) (int, error) {
	return j.write(ctx, StartOfDay(from), StartOfDay(to), true)
}

func (j *Job) write(ctx context.Context, from, to time.Time, overwrite bool) (int, error) {
	written := 0
	for day := from; !day.After(to); day = day.AddDate(0, 0, 1) {
		row, err := j.Collector.Collect(ctx, day)
		if err != nil {
			return written, err
		}
		saved, err := j.Store.Save(ctx, row, overwrite)
		if err != nil {
			return written, err
		}
		if saved {
			written++
		}
	}
	return written, nil
}

func (j *Job) Run(ctx context.Context, every time.Duration) {
	run := func() {
		n, err := j.RunOnce(ctx)
		if err != nil && ctx.Err() == nil {
			j.Log.ErrorContext(ctx, "daily stats job failed", "error", err)
			return
		}
		if n > 0 {
			j.Log.InfoContext(ctx, "daily stats written", "days", n)
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
