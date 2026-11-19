package stats

import (
	"encoding/json"
	"time"
)

type Kind int

const (
	Flow Kind = iota
	Level
	Average
	Median
)

type Metric struct {
	Name       string
	Kind       Kind
	Moderation bool
	Snapshot   bool
}

var Metrics = []Metric{
	{Name: "users_total", Kind: Level},
	{Name: "users_new", Kind: Flow},
	{Name: "users_active_day", Kind: Average, Snapshot: true},
	{Name: "users_active_30d", Kind: Level, Snapshot: true},
	{Name: "messages", Kind: Flow},
	{Name: "messages_text", Kind: Flow},
	{Name: "messages_file", Kind: Flow},
	{Name: "messages_voice", Kind: Flow},
	{Name: "messages_sticker", Kind: Flow},
	{Name: "files_sent", Kind: Flow},
	{Name: "direct_chats_total", Kind: Level},
	{Name: "direct_chats_new", Kind: Flow},
	{Name: "groups_total", Kind: Level},
	{Name: "groups_new", Kind: Flow},
	{Name: "calls", Kind: Flow},
	{Name: "call_minutes", Kind: Flow},
	{Name: "storage_bytes", Kind: Level, Snapshot: true},
	{Name: "messenger_db_bytes", Kind: Level, Snapshot: true},
	{Name: "moderation_db_bytes", Kind: Level, Snapshot: true},
	{Name: "media_jobs_waiting", Kind: Level, Snapshot: true},
	{Name: "reports_total", Kind: Flow, Moderation: true},
	{Name: "pack_reports_total", Kind: Flow, Moderation: true},
	{Name: "decisions_total", Kind: Flow, Moderation: true},
	{Name: "median_decision_seconds", Kind: Median, Moderation: true},
	{Name: "reversals", Kind: Flow, Moderation: true},
}

var Breakdowns = []string{"reports", "pack_reports", "decisions"}

type Row struct {
	Day       time.Time
	Values    map[string]*int64
	Breakdown map[string]map[string]int64
	Late      bool
}

func NewRow(day time.Time) Row {
	return Row{Day: day, Values: map[string]*int64{}, Breakdown: map[string]map[string]int64{}}
}

func (r Row) Set(name string, v int64) { r.Values[name] = &v }

func (r Row) Get(name string) (int64, bool) {
	v := r.Values[name]
	if v == nil {
		return 0, false
	}
	return *v, true
}

func (r Row) Only(moderation bool) Row {
	out := NewRow(r.Day)
	out.Late = r.Late
	for _, m := range Metrics {
		if !moderation || m.Moderation {
			out.Values[m.Name] = r.Values[m.Name]
		}
	}
	for _, b := range Breakdowns {
		if v, ok := r.Breakdown[b]; ok {
			out.Breakdown[b] = v
		}
	}
	return out
}

func (r Row) MarshalJSON() ([]byte, error) {
	out := map[string]any{"day": r.Day.Format(time.DateOnly), "late": r.Late}
	for name, v := range r.Values {
		out[name] = v
	}
	for name, v := range r.Breakdown {
		out[name] = v
	}
	return json.Marshal(out)
}

func StartOfDay(t time.Time) time.Time {
	y, m, d := t.UTC().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}
