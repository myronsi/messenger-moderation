package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/pquerna/otp/totp"

	"github.com/myronsi/messenger-moderation/internal/staff"
	"github.com/myronsi/messenger-moderation/internal/testdb"
)

const origin = "https://moderation.example.com"

type fixture struct {
	handler http.Handler
	admin   staff.Created
	mod     staff.Created
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	pool, _ := testdb.New(t)
	svc, err := staff.NewService(pool, []byte("0123456789abcdef0123456789abcdef"), 12*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	admin, err := svc.Create(ctx, staff.NewStaff{Username: "operator", Password: "a long enough password", Role: staff.RoleAdmin})
	if err != nil {
		t.Fatal(err)
	}
	signed := time.Now()
	mod, err := svc.Create(ctx, staff.NewStaff{Username: "helper", Password: "another long password", Role: staff.RoleModerator, ConfidentialityOn: &signed})
	if err != nil {
		t.Fatal(err)
	}
	console := fstest.MapFS{
		"index.html":      {Data: []byte("<!doctype html><title>console</title>")},
		"assets/app-1.js": {Data: []byte("console.log(1)")},
	}
	handler := New(Deps{
		Staff: svc, PublicOrigin: origin, Console: console,
		Register: func(mux *http.ServeMux, s *Server) {
			mux.Handle("GET /api/admin-only", s.RequireAdmin(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
			})))
		},
	})
	return fixture{handler: handler, admin: admin, mod: mod}
}

func (f fixture) do(t *testing.T, method, target, body string, cookie *http.Cookie, withOrigin bool) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if withOrigin {
		req.Header.Set("Origin", origin)
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	f.handler.ServeHTTP(rec, req)
	return rec
}

func (f fixture) signIn(t *testing.T, c staff.Created, password string) *http.Cookie {
	t.Helper()
	code, err := totp.GenerateCode(c.TOTPSecret, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	body := `{"username":"` + c.Staff.Username + `","password":"` + password + `","code":"` + code + `"}`
	rec := f.do(t, http.MethodPost, "/api/auth/login", body, nil, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("login status = %d, body %s", rec.Code, rec.Body)
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == SessionCookie {
			if !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteStrictMode {
				t.Fatalf("cookie flags = %+v", c)
			}
			return c
		}
	}
	t.Fatal("no session cookie")
	return nil
}

func TestLoginMeLogout(t *testing.T) {
	f := newFixture(t)

	if rec := f.do(t, http.MethodGet, "/api/me", "", nil, false); rec.Code != http.StatusUnauthorized {
		t.Fatalf("me without session = %d", rec.Code)
	}
	if rec := f.do(t, http.MethodPost, "/api/auth/login", `{"username":"operator","password":"a long enough password","code":"000000"}`, nil, true); rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong code = %d", rec.Code)
	}

	cookie := f.signIn(t, f.admin, "a long enough password")
	rec := f.do(t, http.MethodGet, "/api/me", "", cookie, false)
	if rec.Code != http.StatusOK {
		t.Fatalf("me = %d %s", rec.Code, rec.Body)
	}
	var me struct {
		Staff staff.Staff `json:"staff"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &me); err != nil || me.Staff.Username != "operator" || me.Staff.Role != staff.RoleAdmin {
		t.Fatalf("me body = %s, %v", rec.Body, err)
	}
	if rec.Header().Get("Cache-Control") != "no-store" || rec.Header().Get("Content-Security-Policy") == "" {
		t.Fatalf("headers = %v", rec.Header())
	}

	if rec := f.do(t, http.MethodPost, "/api/auth/logout", "", cookie, true); rec.Code != http.StatusNoContent {
		t.Fatalf("logout = %d", rec.Code)
	}
	if rec := f.do(t, http.MethodGet, "/api/me", "", cookie, false); rec.Code != http.StatusUnauthorized {
		t.Fatalf("me after logout = %d", rec.Code)
	}
}

func TestStateChangesNeedTheConsoleOrigin(t *testing.T) {
	f := newFixture(t)
	rec := f.do(t, http.MethodPost, "/api/auth/login", `{"username":"operator","password":"x","code":"1"}`, nil, false)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("login without origin = %d", rec.Code)
	}
}

func TestAdminOnlyRoutes(t *testing.T) {
	f := newFixture(t)
	if rec := f.do(t, http.MethodGet, "/api/admin-only", "", f.signIn(t, f.mod, "another long password"), false); rec.Code != http.StatusForbidden {
		t.Fatalf("moderator = %d", rec.Code)
	}
	if rec := f.do(t, http.MethodGet, "/api/admin-only", "", f.signIn(t, f.admin, "a long enough password"), false); rec.Code != http.StatusOK {
		t.Fatalf("admin = %d", rec.Code)
	}
}

func TestConsoleIsServedWithFallback(t *testing.T) {
	f := newFixture(t)
	if rec := f.do(t, http.MethodGet, "/queue/42", "", nil, false); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "<title>console") {
		t.Fatalf("fallback = %d %s", rec.Code, rec.Body)
	}
	rec := f.do(t, http.MethodGet, "/assets/app-1.js", "", nil, false)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Header().Get("Cache-Control"), "immutable") {
		t.Fatalf("asset = %d %v", rec.Code, rec.Header())
	}
	if rec := f.do(t, http.MethodGet, "/api/unknown", "", nil, false); rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "not_found") {
		t.Fatalf("unknown api = %d %s", rec.Code, rec.Body)
	}
}

func TestMetaIsPublic(t *testing.T) {
	f := newFixture(t)
	rec := f.do(t, http.MethodGet, "/api/meta", "", nil, false)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"environment":"development"`) {
		t.Fatalf("meta = %d %s", rec.Code, rec.Body)
	}
}
