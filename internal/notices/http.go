package notices

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/myronsi/messenger-moderation/internal/httpserver"
)

type handler struct {
	svc *Service
	log *slog.Logger
}

func Register(mux *http.ServeMux, s *httpserver.Server, svc *Service, log *slog.Logger) {
	if log == nil {
		log = slog.Default()
	}
	h := &handler{svc: svc, log: log}
	mux.Handle("GET /api/notices/settings", s.RequireStaff(http.HandlerFunc(h.settings)))
	mux.Handle("POST /api/notices", s.RequireStaff(http.HandlerFunc(h.create)))
	mux.Handle("POST /api/cases/{id}/notice-decision", s.RequireStaff(http.HandlerFunc(h.decide)))
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(io.LimitReader(r.Body, 32<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		httpserver.WriteError(w, http.StatusBadRequest, "invalid_request", "the request body is not valid JSON")
		return false
	}
	return true
}

func (h *handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrInvalid):
		httpserver.WriteError(w, http.StatusBadRequest, "invalid_request", err.Error())
	case errors.Is(err, ErrNotFound):
		httpserver.WriteError(w, http.StatusNotFound, "not_found", "no case with this ID")
	default:
		h.log.ErrorContext(r.Context(), "notice request failed", "route", r.URL.Path, "error", err)
		httpserver.WriteError(w, http.StatusInternalServerError, "internal_error", "the notice could not be saved")
	}
}

func (h *handler) settings(w http.ResponseWriter, _ *http.Request) {
	httpserver.WriteJSON(w, http.StatusOK, map[string]bool{"mail_enabled": h.svc.MailEnabled()})
}

func (h *handler) create(w http.ResponseWriter, r *http.Request) {
	var in Input
	if !decode(w, r, &in) {
		return
	}
	st, _ := httpserver.StaffFrom(r.Context())
	id, err := h.svc.Create(r.Context(), st, in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpserver.WriteJSON(w, http.StatusCreated, map[string]string{"id": strconv.FormatInt(id, 10)})
}

func (h *handler) decide(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		httpserver.WriteError(w, http.StatusNotFound, "not_found", "no case with this ID")
		return
	}
	var d Decision
	if !decode(w, r, &d) {
		return
	}
	st, _ := httpserver.StaffFrom(r.Context())
	state, err := h.svc.Decide(r.Context(), st, id, d)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpserver.WriteJSON(w, http.StatusOK, map[string]string{"mail_state": state})
}
