package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/myronsi/messenger-moderation/internal/staff"
)

const SessionCookie = "mod_session"

type Deps struct {
	Staff        *staff.Service
	PublicOrigin string
	TrustProxy   bool
	Environment  string
	Version      string
	Console      fs.FS
	Ready        func(ctx context.Context) error
	Log          *slog.Logger
	Register     func(mux *http.ServeMux, s *Server)
}

type Server struct {
	deps Deps
	mux  *http.ServeMux
}

type ctxKey struct{}

func New(d Deps) http.Handler {
	if d.Log == nil {
		d.Log = slog.Default()
	}
	s := &Server{deps: d, mux: http.NewServeMux()}
	s.mux.HandleFunc("GET /healthz", s.health)
	s.mux.HandleFunc("GET /api/meta", s.meta)
	s.mux.HandleFunc("POST /api/auth/login", s.login)
	s.mux.HandleFunc("POST /api/auth/logout", s.logout)
	s.mux.Handle("GET /api/me", s.RequireStaff(http.HandlerFunc(s.me)))
	s.mux.HandleFunc("/api/", func(w http.ResponseWriter, _ *http.Request) {
		WriteError(w, http.StatusNotFound, "not_found", "unknown endpoint")
	})
	if d.Register != nil {
		d.Register(s.mux, s)
	}
	s.mux.Handle("/", s.console())
	return securityHeaders(s.sameOrigin(s.mux))
}

func StaffFrom(ctx context.Context) (staff.Staff, bool) {
	st, ok := ctx.Value(ctxKey{}).(staff.Staff)
	return st, ok
}

func (s *Server) RequireStaff(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(SessionCookie)
		if err != nil {
			WriteError(w, http.StatusUnauthorized, "unauthenticated", "sign in first")
			return
		}
		st, err := s.deps.Staff.Authenticate(r.Context(), cookie.Value)
		if errors.Is(err, staff.ErrUnauthenticated) {
			s.clearCookie(w)
			WriteError(w, http.StatusUnauthorized, "unauthenticated", "your session has ended, sign in again")
			return
		}
		if err != nil {
			s.internalError(w, r, err)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, st)))
	})
}

func (s *Server) RequireAdmin(next http.Handler) http.Handler {
	return s.RequireStaff(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if st, _ := StaffFrom(r.Context()); !st.IsAdmin() {
			WriteError(w, http.StatusForbidden, "forbidden", "only admins can do this")
			return
		}
		next.ServeHTTP(w, r)
	}))
}

func (s *Server) ClientIP(r *http.Request) string {
	if s.deps.TrustProxy {
		if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
			first, _, _ := strings.Cut(fwd, ",")
			return strings.TrimSpace(first)
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	if s.deps.Ready != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := s.deps.Ready(ctx); err != nil {
			WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unavailable"})
			return
		}
	}
	WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Code     string `json:"code"`
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_request", "send username, password and code as JSON")
		return
	}
	session, err := s.deps.Staff.Login(r.Context(), req.Username, req.Password, req.Code, s.ClientIP(r), r.UserAgent())
	switch {
	case errors.Is(err, staff.ErrTooManyAttempts):
		WriteError(w, http.StatusTooManyRequests, "too_many_attempts", err.Error())
		return
	case errors.Is(err, staff.ErrInvalidLogin):
		WriteError(w, http.StatusUnauthorized, "invalid_login", err.Error())
		return
	case err != nil:
		s.internalError(w, r, err)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: SessionCookie, Value: session.Token, Path: "/", Expires: session.ExpiresAt,
		HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode,
	})
	WriteJSON(w, http.StatusOK, map[string]any{"staff": session.Staff, "expires_at": session.ExpiresAt})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(SessionCookie); err == nil {
		if err := s.deps.Staff.Logout(r.Context(), cookie.Value); err != nil {
			s.internalError(w, r, err)
			return
		}
	}
	s.clearCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) meta(w http.ResponseWriter, _ *http.Request) {
	environment := s.deps.Environment
	if environment == "" {
		environment = "development"
	}
	WriteJSON(w, http.StatusOK, map[string]string{"environment": environment, "version": s.deps.Version})
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	st, _ := StaffFrom(r.Context())
	WriteJSON(w, http.StatusOK, map[string]any{"staff": st})
}

func (s *Server) clearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: SessionCookie, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode,
	})
}

func (s *Server) sameOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
		default:
			if r.Header.Get("Origin") != s.deps.PublicOrigin {
				WriteError(w, http.StatusForbidden, "bad_origin", "requests must come from the console")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) console() http.Handler {
	if s.deps.Console == nil {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "console not built", http.StatusNotFound)
		})
	}
	files := http.FileServerFS(s.deps.Console)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name != "" && name != "index.html" {
			if info, err := fs.Stat(s.deps.Console, name); err == nil && !info.IsDir() {
				if strings.HasPrefix(name, "assets/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				}
				files.ServeHTTP(w, r)
				return
			}
		}
		index, err := fs.ReadFile(s.deps.Console, "index.html")
		if err != nil {
			http.Error(w, "console not built", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(index)
	})
}

func (s *Server) internalError(w http.ResponseWriter, r *http.Request, err error) {
	s.deps.Log.ErrorContext(r.Context(), "request failed", "method", r.Method, "route", r.URL.Path, "error", err)
	WriteError(w, http.StatusInternalServerError, "internal_error", "something went wrong")
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data: blob:; style-src 'self' 'unsafe-inline'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			h.Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}

func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func WriteError(w http.ResponseWriter, status int, code, message string) {
	WriteJSON(w, status, map[string]string{"error": code, "message": message})
}
