package content

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/myronsi/messenger-moderation/internal/httpserver"
	"github.com/myronsi/messenger-moderation/internal/staff"
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
	mux.Handle("GET /api/content-reports", s.RequireStaff(http.HandlerFunc(h.list)))
	mux.Handle("GET /api/content-reports/{id}", s.RequireStaff(http.HandlerFunc(h.get)))
	mux.Handle("POST /api/content-reports/{id}/remove", s.RequireStaff(http.HandlerFunc(h.remove)))
	mux.Handle("POST /api/content-reports/{id}/dismiss", s.RequireStaff(http.HandlerFunc(h.dismiss)))
}

func current(r *http.Request) staff.Staff {
	st, _ := httpserver.StaffFrom(r.Context())
	return st
}

func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		httpserver.WriteError(w, http.StatusNotFound, "not_found", "no report with this ID")
		return 0, false
	}
	return id, true
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(io.LimitReader(r.Body, 16<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		httpserver.WriteError(w, http.StatusBadRequest, "invalid_request", "the request body is not valid JSON")
		return false
	}
	return true
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrInvalid):
		httpserver.WriteError(w, http.StatusBadRequest, "invalid_request", err.Error())
	case errors.Is(err, ErrNotFound):
		httpserver.WriteError(w, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, ErrUnavailable):
		httpserver.WriteError(w, http.StatusServiceUnavailable, "messenger_unavailable", "Messenger is not connected")
	default:
		h.log.ErrorContext(r.Context(), "content report request failed", "route", r.URL.Path, "error", err)
		httpserver.WriteError(w, http.StatusBadGateway, "decision_failed", "the request could not be completed; reload the report before trying again")
	}
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	if state != "" && state != "open" && state != "decided" {
		httpserver.WriteError(w, http.StatusBadRequest, "invalid_request", "state must be open or decided")
		return
	}
	list, err := h.svc.List(r.Context(), current(r), state == "decided")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpserver.WriteJSON(w, http.StatusOK, map[string]any{"reports": list})
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	d, err := h.svc.Get(r.Context(), current(r), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpserver.WriteJSON(w, http.StatusOK, d)
}

func (h *Handler) remove(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var body Reason
	if !decode(w, r, &body) {
		return
	}
	res, err := h.svc.Remove(r.Context(), current(r), id, body)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpserver.WriteJSON(w, http.StatusOK, res)
}

func (h *Handler) dismiss(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var body struct {
		Note string `json:"note"`
	}
	if !decode(w, r, &body) {
		return
	}
	if err := h.svc.Dismiss(r.Context(), current(r), id, body.Note); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
