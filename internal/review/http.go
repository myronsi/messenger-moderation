package review

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/myronsi/messenger-moderation/internal/httpserver"
)

type Handler struct {
	svc *Service
	log *slog.Logger
}

func Register(mux *http.ServeMux, s *httpserver.Server, svc *Service, log *slog.Logger) {
	if log == nil {
		log = slog.Default()
	}
	h := &Handler{svc: svc, log: log}
	mux.Handle("GET /api/queue", s.RequireStaff(http.HandlerFunc(h.queue)))
	mux.Handle("GET /api/users", s.RequireStaff(http.HandlerFunc(h.search)))
	mux.Handle("GET /api/users/{id}", s.RequireStaff(http.HandlerFunc(h.account)))
}

func (h *Handler) queue(w http.ResponseWriter, r *http.Request) {
	q, err := parseQueueQuery(r.URL.Query())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	st, _ := httpserver.StaffFrom(r.Context())
	page, err := h.svc.Queue(r.Context(), st.ID, q)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpserver.WriteJSON(w, http.StatusOK, page)
}

func (h *Handler) account(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		httpserver.WriteError(w, http.StatusNotFound, "not_found", "no account with this ID")
		return
	}
	st, _ := httpserver.StaffFrom(r.Context())
	a, err := h.svc.Account(r.Context(), st.ID, id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpserver.WriteJSON(w, http.StatusOK, a)
}

func (h *Handler) search(w http.ResponseWriter, r *http.Request) {
	st, _ := httpserver.StaffFrom(r.Context())
	hits, err := h.svc.Search(r.Context(), st.ID, r.URL.Query().Get("q"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpserver.WriteJSON(w, http.StatusOK, map[string]any{"users": hits})
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrInvalid):
		httpserver.WriteError(w, http.StatusBadRequest, "invalid_request", err.Error())
	case errors.Is(err, ErrNotFound):
		httpserver.WriteError(w, http.StatusNotFound, "not_found", "no account with this ID")
	case errors.Is(err, ErrUnavailable):
		httpserver.WriteError(w, http.StatusServiceUnavailable, "messenger_unavailable", "the Messenger database is not connected")
	default:
		h.log.ErrorContext(r.Context(), "review request failed", "route", r.URL.Path, "error", err)
		httpserver.WriteError(w, http.StatusInternalServerError, "internal_error", "something went wrong")
	}
}

func parseQueueQuery(v url.Values) (QueueQuery, error) {
	var q QueueQuery
	var err error
	if q.Statuses, err = parseList(v, "status", Statuses); err != nil {
		return q, err
	}
	if q.Reasons, err = parseList(v, "reason", Reasons); err != nil {
		return q, err
	}
	if q.OpenedAfter, err = parseTime(v, "opened_after"); err != nil {
		return q, err
	}
	if q.OpenedBefore, err = parseTime(v, "opened_before"); err != nil {
		return q, err
	}
	if raw := v.Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > MaxQueueLimit {
			return q, fmt.Errorf("%w: limit must be 1 to %d", ErrInvalid, MaxQueueLimit)
		}
		q.Limit = n
	}
	if raw := v.Get("cursor"); raw != "" {
		c, err := DecodeCursor(raw)
		if err != nil {
			return q, err
		}
		q.After = &c
	}
	return q, nil
}

func parseList(v url.Values, name string, allowed []string) ([]string, error) {
	var out []string
	for _, raw := range v[name] {
		for _, item := range strings.Split(raw, ",") {
			item = strings.TrimSpace(item)
			if item == "" {
				continue
			}
			if !slices.Contains(allowed, item) {
				return nil, fmt.Errorf("%w: unknown %s %q", ErrInvalid, name, item)
			}
			if !slices.Contains(out, item) {
				out = append(out, item)
			}
		}
	}
	return out, nil
}

func parseTime(v url.Values, name string) (*time.Time, error) {
	raw := v.Get(name)
	if raw == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil, fmt.Errorf("%w: %s must be an RFC 3339 time", ErrInvalid, name)
	}
	return &t, nil
}
