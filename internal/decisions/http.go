package decisions

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
	mux.Handle("POST /api/users/{id}/status", s.RequireStaff(http.HandlerFunc(h.setStatus)))
	mux.Handle("POST /api/users/{id}/reports/dismiss", s.RequireStaff(http.HandlerFunc(h.dismiss)))
	mux.Handle("DELETE /api/users/{id}", s.RequireAdmin(http.HandlerFunc(h.deleteAccount)))
	mux.Handle("GET /api/cases", s.RequireStaff(http.HandlerFunc(h.cases)))
	mux.Handle("POST /api/cases/{id}/confirm", s.RequireStaff(http.HandlerFunc(h.confirm)))
	mux.Handle("POST /api/cases/{id}/reverse", s.RequireStaff(http.HandlerFunc(h.reverse)))
	mux.Handle("GET /api/packs", s.RequireStaff(http.HandlerFunc(h.packs)))
	mux.Handle("GET /api/packs/{kind}/{id}", s.RequireStaff(http.HandlerFunc(h.pack)))
	mux.Handle("POST /api/packs/{kind}/{id}/hide", s.RequireStaff(h.setHidden(true)))
	mux.Handle("POST /api/packs/{kind}/{id}/unhide", s.RequireStaff(h.setHidden(false)))
	mux.Handle("GET /api/packs/{kind}/{id}/images/{attachment}", s.RequireStaff(http.HandlerFunc(h.image)))
}

func current(r *http.Request) staff.Staff {
	st, _ := httpserver.StaffFrom(r.Context())
	return st
}

func pathID(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	return id, err == nil && id > 0
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
	case errors.Is(err, ErrForbidden):
		httpserver.WriteError(w, http.StatusForbidden, "forbidden", err.Error())
	case errors.Is(err, ErrNotFound):
		httpserver.WriteError(w, http.StatusNotFound, "not_found", "not found")
	case errors.Is(err, ErrUnavailable):
		httpserver.WriteError(w, http.StatusServiceUnavailable, "messenger_unavailable", "Messenger is not connected")
	default:
		h.log.ErrorContext(r.Context(), "decision request failed", "route", r.URL.Path, "error", err)
		httpserver.WriteError(w, http.StatusBadGateway, "decision_failed", "the decision could not be completed; check the account before trying again")
	}
}

type statusBody struct {
	Status string `json:"status"`
	Reason
}

func (h *Handler) setStatus(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		httpserver.WriteError(w, http.StatusNotFound, "not_found", "no account with this ID")
		return
	}
	var body statusBody
	if !decode(w, r, &body) {
		return
	}
	change, err := h.svc.SetStatus(r.Context(), current(r), id, body.Status, body.Reason)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpserver.WriteJSON(w, http.StatusOK, change)
}

type dismissBody struct {
	ReporterIDs *[]string `json:"reporter_ids"`
	Reason
}

func (h *Handler) dismiss(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		httpserver.WriteError(w, http.StatusNotFound, "not_found", "no account with this ID")
		return
	}
	var body dismissBody
	if !decode(w, r, &body) {
		return
	}
	var reporters []int64
	if body.ReporterIDs != nil {
		reporters = make([]int64, 0, len(*body.ReporterIDs))
		for _, raw := range *body.ReporterIDs {
			n, err := strconv.ParseInt(raw, 10, 64)
			if err != nil || n <= 0 {
				httpserver.WriteError(w, http.StatusBadRequest, "invalid_request", "reporter_ids must be account IDs")
				return
			}
			reporters = append(reporters, n)
		}
	}
	res, err := h.svc.DismissReports(r.Context(), current(r), id, reporters, body.Reason)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpserver.WriteJSON(w, http.StatusOK, res)
}

func (h *Handler) deleteAccount(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		httpserver.WriteError(w, http.StatusNotFound, "not_found", "no account with this ID")
		return
	}
	var body Reason
	if !decode(w, r, &body) {
		return
	}
	if err := h.svc.DeleteAccount(r.Context(), current(r), id, body); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) cases(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	if state != "" && state != "open" && state != "decided" {
		httpserver.WriteError(w, http.StatusBadRequest, "invalid_request", "state must be open or decided")
		return
	}
	list, err := h.svc.Cases(r.Context(), current(r), state != "decided")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpserver.WriteJSON(w, http.StatusOK, map[string]any{"cases": list})
}

func (h *Handler) confirm(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		httpserver.WriteError(w, http.StatusNotFound, "not_found", "no case with this ID")
		return
	}
	var body Reason
	if !decode(w, r, &body) {
		return
	}
	if err := h.svc.ConfirmCase(r.Context(), current(r), id, body); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) reverse(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		httpserver.WriteError(w, http.StatusNotFound, "not_found", "no case with this ID")
		return
	}
	var body statusBody
	if !decode(w, r, &body) {
		return
	}
	if err := h.svc.ReverseCase(r.Context(), current(r), id, body.Status, body.Reason); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) packs(w http.ResponseWriter, r *http.Request) {
	list, err := h.svc.ReportedPacks(r.Context(), current(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpserver.WriteJSON(w, http.StatusOK, map[string]any{"packs": list})
}

func (h *Handler) pack(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		httpserver.WriteError(w, http.StatusNotFound, "not_found", "no pack with this ID")
		return
	}
	p, err := h.svc.Pack(r.Context(), current(r), r.PathValue("kind"), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpserver.WriteJSON(w, http.StatusOK, p)
}

func (h *Handler) setHidden(hidden bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(r)
		if !ok {
			httpserver.WriteError(w, http.StatusNotFound, "not_found", "no pack with this ID")
			return
		}
		var body Reason
		if !decode(w, r, &body) {
			return
		}
		changed, err := h.svc.SetPackHidden(r.Context(), current(r), r.PathValue("kind"), id, hidden, body)
		if err != nil {
			h.fail(w, r, err)
			return
		}
		httpserver.WriteJSON(w, http.StatusOK, map[string]bool{"changed": changed})
	})
}

func (h *Handler) image(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	resp, err := h.svc.PackImage(r.Context(), r.PathValue("kind"), id, r.PathValue("attachment"))
	if errors.Is(err, ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		h.fail(w, r, err)
		return
	}
	defer func() { _ = resp.Body.Close() }()
	CopyImage(w, resp)
}
