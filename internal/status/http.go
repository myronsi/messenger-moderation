package status

import (
	"net/http"
	"slices"
	"time"

	"github.com/myronsi/messenger-moderation/internal/httpserver"
)

func Register(mux *http.ServeMux, s *httpserver.Server, reporter *Reporter, publicOrigins []string) {
	mux.HandleFunc("GET /status.json", func(w http.ResponseWriter, r *http.Request) {
		if origin := r.Header.Get("Origin"); origin != "" && slices.Contains(publicOrigins, origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
		}
		p, err := reporter.Public(r.Context(), time.Now())
		if err != nil {
			httpserver.WriteError(w, http.StatusServiceUnavailable, "unavailable", "status is not available right now")
			return
		}
		w.Header().Set("Cache-Control", "public, max-age=30")
		httpserver.WriteJSON(w, http.StatusOK, p)
	})
	mux.Handle("GET /api/status", s.RequireAdmin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		o, err := reporter.Overview(r.Context(), time.Now())
		if err != nil {
			httpserver.WriteError(w, http.StatusInternalServerError, "internal_error", "status could not be loaded")
			return
		}
		httpserver.WriteJSON(w, http.StatusOK, o)
	})))
}
