package stats

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/myronsi/messenger-moderation/internal/httpserver"
)

const (
	DefaultDays = 30
	MaxDays     = 3660
)

type Handlers struct {
	Store     *Store
	Collector *Collector
	Log       *slog.Logger
	Now       func() time.Time
}

func (h *Handlers) Register(mux *http.ServeMux, s *httpserver.Server) {
	mux.Handle("GET /api/stats", s.RequireStaff(http.HandlerFunc(h.list)))
	mux.Handle("GET /api/stats/today", s.RequireStaff(http.HandlerFunc(h.today)))
}

func (h *Handlers) now() time.Time {
	if h.Now != nil {
		return h.Now()
	}
	return time.Now()
}

func moderationOnly(r *http.Request) bool {
	st, _ := httpserver.StaffFrom(r.Context())
	return !st.IsAdmin()
}

func scope(only bool) string {
	if only {
		return "moderation"
	}
	return "all"
}

func filter(rows []Row, only bool) []Row {
	out := make([]Row, len(rows))
	for i, r := range rows {
		out[i] = r.Only(only)
	}
	return out
}

func (h *Handlers) list(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	group, ok := ParseGrouping(q.Get("group"))
	if !ok {
		httpserver.WriteError(w, http.StatusBadRequest, "invalid_request", "group must be day, week or month")
		return
	}
	to := StartOfDay(h.now()).AddDate(0, 0, -1)
	if v := q.Get("to"); v != "" {
		t, err := time.Parse(time.DateOnly, v)
		if err != nil {
			httpserver.WriteError(w, http.StatusBadRequest, "invalid_request", "to must be a date (YYYY-MM-DD)")
			return
		}
		to = t
	}
	from := to.AddDate(0, 0, 1-DefaultDays)
	if v := q.Get("from"); v != "" {
		t, err := time.Parse(time.DateOnly, v)
		if err != nil {
			httpserver.WriteError(w, http.StatusBadRequest, "invalid_request", "from must be a date (YYYY-MM-DD)")
			return
		}
		from = t
	}
	if from.After(to) || to.Sub(from) > MaxDays*24*time.Hour {
		httpserver.WriteError(w, http.StatusBadRequest, "invalid_request", fmt.Sprintf("from must be before to, at most %d days apart", MaxDays))
		return
	}
	format := q.Get("format")
	if format != "" && format != "json" && format != "csv" {
		httpserver.WriteError(w, http.StatusBadRequest, "invalid_request", "format must be json or csv")
		return
	}
	rows, err := h.Store.Range(r.Context(), from, to)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	only := moderationOnly(r)
	rows = filter(Combine(rows, group), only)
	if format == "csv" {
		name := fmt.Sprintf("messenger-%s-stats-%s-%s-by-%s.csv", scope(only), from.Format(time.DateOnly), to.Format(time.DateOnly), group)
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
		if err := WriteCSV(w, rows, only); err != nil {
			h.Log.WarnContext(r.Context(), "writing stats csv", "error", err)
		}
		return
	}
	httpserver.WriteJSON(w, http.StatusOK, map[string]any{
		"from": from.Format(time.DateOnly), "to": to.Format(time.DateOnly), "group": group, "scope": scope(only), "rows": rows,
	})
}

func (h *Handlers) today(w http.ResponseWriter, r *http.Request) {
	if h.Collector == nil || h.Collector.Messenger == nil {
		httpserver.WriteError(w, http.StatusServiceUnavailable, "unavailable", "the Messenger database is not connected")
		return
	}
	only := moderationOnly(r)
	today := StartOfDay(h.now())
	live, err := h.Collector.Collect(r.Context(), today)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	yesterday := today.AddDate(0, 0, -1)
	stored, err := h.Store.Range(r.Context(), yesterday, yesterday)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	body := map[string]any{"scope": scope(only), "today": live.Only(only), "yesterday": nil, "as_of": h.now().UTC()}
	if len(stored) == 1 {
		body["yesterday"] = stored[0].Only(only)
	}
	httpserver.WriteJSON(w, http.StatusOK, body)
}

func (h *Handlers) fail(w http.ResponseWriter, r *http.Request, err error) {
	h.Log.ErrorContext(r.Context(), "stats request failed", "route", r.URL.Path, "error", err)
	httpserver.WriteError(w, http.StatusInternalServerError, "internal_error", "something went wrong")
}
