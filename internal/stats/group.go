package stats

import (
	"encoding/csv"
	"io"
	"slices"
	"strconv"
	"time"
)

type Grouping string

const (
	ByDay   Grouping = "day"
	ByWeek  Grouping = "week"
	ByMonth Grouping = "month"
)

func ParseGrouping(s string) (Grouping, bool) {
	switch g := Grouping(s); g {
	case "":
		return ByDay, true
	case ByDay, ByWeek, ByMonth:
		return g, true
	}
	return "", false
}

func (g Grouping) Start(day time.Time) time.Time {
	day = StartOfDay(day)
	switch g {
	case ByWeek:
		offset := (int(day.Weekday()) + 6) % 7
		return day.AddDate(0, 0, -offset)
	case ByMonth:
		return time.Date(day.Year(), day.Month(), 1, 0, 0, 0, 0, time.UTC)
	}
	return day
}

func Combine(rows []Row, g Grouping) []Row {
	if g == ByDay {
		return rows
	}
	out := []Row{}
	for start := 0; start < len(rows); {
		bucket := g.Start(rows[start].Day)
		end := start
		for end < len(rows) && g.Start(rows[end].Day).Equal(bucket) {
			end++
		}
		out = append(out, combine(bucket, rows[start:end]))
		start = end
	}
	return out
}

func combine(day time.Time, rows []Row) Row {
	r := NewRow(day)
	for _, row := range rows {
		r.Late = r.Late || row.Late
	}
	for _, m := range Metrics {
		var sum, count int64
		var last *int64
		for _, row := range rows {
			if v := row.Values[m.Name]; v != nil {
				sum += *v
				count++
				last = v
			}
		}
		if count == 0 {
			r.Values[m.Name] = nil
			continue
		}
		switch m.Kind {
		case Flow:
			r.Set(m.Name, sum)
		case Level:
			r.Set(m.Name, *last)
		case Average, Median:
			r.Set(m.Name, (sum+count/2)/count)
		}
	}
	for _, b := range Breakdowns {
		var merged map[string]int64
		for _, row := range rows {
			part, ok := row.Breakdown[b]
			if !ok {
				continue
			}
			if merged == nil {
				merged = map[string]int64{}
			}
			for k, v := range part {
				merged[k] += v
			}
		}
		if merged != nil {
			r.Breakdown[b] = merged
		}
	}
	return r
}

func WriteCSV(w io.Writer, rows []Row, moderationOnly bool) error {
	header := []string{"period"}
	var metrics []string
	for _, m := range Metrics {
		if !moderationOnly || m.Moderation {
			metrics = append(metrics, m.Name)
		}
	}
	header = append(header, metrics...)
	type column struct{ breakdown, key string }
	var extra []column
	for _, b := range Breakdowns {
		keys := map[string]bool{}
		for _, r := range rows {
			for k := range r.Breakdown[b] {
				keys[k] = true
			}
		}
		sorted := make([]string, 0, len(keys))
		for k := range keys {
			sorted = append(sorted, k)
		}
		slices.Sort(sorted)
		for _, k := range sorted {
			extra = append(extra, column{b, k})
			header = append(header, b+"_"+k)
		}
	}
	header = append(header, "late")
	cw := csv.NewWriter(w)
	if err := cw.Write(header); err != nil {
		return err
	}
	for _, r := range rows {
		line := []string{r.Day.Format(time.DateOnly)}
		for _, name := range metrics {
			v, ok := r.Get(name)
			if !ok {
				line = append(line, "")
				continue
			}
			line = append(line, strconv.FormatInt(v, 10))
		}
		for _, c := range extra {
			part, ok := r.Breakdown[c.breakdown]
			if !ok {
				line = append(line, "")
				continue
			}
			line = append(line, strconv.FormatInt(part[c.key], 10))
		}
		line = append(line, strconv.FormatBool(r.Late))
		if err := cw.Write(line); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}
