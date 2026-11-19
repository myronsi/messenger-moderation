package stats_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"

	"github.com/myronsi/messenger-moderation/internal/httpserver"
	"github.com/myronsi/messenger-moderation/internal/staff"
	"github.com/myronsi/messenger-moderation/internal/stats"
	"github.com/myronsi/messenger-moderation/internal/testdb"
)

const origin = "https://moderation.example.com"

type server struct {
	handler http.Handler
	admin   *http.Cookie
	mod     *http.Cookie
}

func newServer(t *testing.T) server {
	t.Helper()
	m := messengerDB(t, true)
	seed(t, m)
	pool, _ := testdb.New(t)
	ctx := context.Background()
	svc, err := staff.NewService(pool, []byte(strings.Repeat("k", 32)), 12*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	admin, err := svc.Create(ctx, staff.NewStaff{Username: "operator", Password: "a long enough password", Role: staff.RoleAdmin})
	if err != nil {
		t.Fatal(err)
	}
	signed := time.Now()
	mod, err := svc.Create(ctx, staff.NewStaff{Username: "helper", Password: "another long password", Role: staff.RoleModerator, ConfidentialityOn: &signed})
	if err != nil {
		t.Fatal(err)
	}
	store := stats.NewStore(pool)
	clock := func() time.Time { return at(24+11, 0) }
	collector := &stats.Collector{Messenger: m, Moderation: store, Now: clock}
	if _, err := (&stats.Job{Collector: &stats.Collector{Messenger: m, Moderation: store, Now: func() time.Time { return at(24, 5) }}, Store: store}).Rerun(ctx, day, day); err != nil {
		t.Fatal(err)
	}
	h := &stats.Handlers{Store: store, Collector: collector, Now: clock}
	s := server{handler: httpserver.New(httpserver.Deps{Staff: svc, PublicOrigin: origin, Register: h.Register})}
	s.admin = s.signIn(t, admin, "a long enough password")
	s.mod = s.signIn(t, mod, "another long password")
	return s
}

func (s server) signIn(t *testing.T, c staff.Created, password string) *http.Cookie {
	t.Helper()
	code, err := totp.GenerateCode(c.TOTPSecret, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"`+c.Staff.Username+`","password":"`+password+`","code":"`+code+`"}`))
	req.Header.Set("Origin", origin)
	rec := httptest.NewRecorder()
	s.handler.ServeHTTP(rec, req)
	for _, ck := range rec.Result().Cookies() {
		if ck.Name == httpserver.SessionCookie {
			return ck
		}
	}
	t.Fatalf("login failed: %d %s", rec.Code, rec.Body)
	return nil
}

func (s server) get(t *testing.T, target string, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	s.handler.ServeHTTP(rec, req)
	return rec
}

func TestStatsEndpointsByRole(t *testing.T) {
	s := newServer(t)
	for _, target := range []string{"/api/stats", "/api/stats/today"} {
		if rec := s.get(t, target, nil); rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s without a session: %d", target, rec.Code)
		}
	}

	var body struct {
		Scope string           `json:"scope"`
		Rows  []map[string]any `json:"rows"`
	}
	rec := s.get(t, "/api/stats", s.admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("admin: %d %s", rec.Code, rec.Body)
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Scope != "all" || len(body.Rows) != 1 || body.Rows[0]["day"] != "2026-11-18" || body.Rows[0]["users_total"] != float64(4) || body.Rows[0]["messages"] != float64(0) {
		t.Fatalf("admin body %+v", body)
	}

	rec = s.get(t, "/api/stats?from=2026-11-01&to=2026-11-30&group=month", s.mod)
	if rec.Code != http.StatusOK {
		t.Fatalf("moderator: %d %s", rec.Code, rec.Body)
	}
	body.Rows = nil
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Scope != "moderation" || len(body.Rows) != 1 || body.Rows[0]["day"] != "2026-11-01" || body.Rows[0]["reports_total"] != float64(3) {
		t.Fatalf("moderator body %+v", body)
	}
	if _, ok := body.Rows[0]["users_total"]; ok {
		t.Fatal("moderators do not see usage numbers")
	}

	rec = s.get(t, "/api/stats?format=csv&from=2026-11-18&to=2026-11-18", s.mod)
	if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/csv") ||
		!strings.Contains(rec.Header().Get("Content-Disposition"), "messenger-moderation-stats-2026-11-18-2026-11-18-by-day.csv") {
		t.Fatalf("csv: %d %v", rec.Code, rec.Header())
	}
	if strings.Contains(rec.Body.String(), "users_total") || !strings.Contains(rec.Body.String(), "2026-11-18,3,2,") {
		t.Fatalf("moderator csv %q", rec.Body)
	}

	for _, bad := range []string{"group=year", "from=yesterday", "from=2026-11-20&to=2026-11-01", "format=xml", "from=2000-01-01&to=2026-11-01"} {
		if rec := s.get(t, "/api/stats?"+bad, s.admin); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: %d", bad, rec.Code)
		}
	}
}

func TestTodayComparesWithYesterday(t *testing.T) {
	s := newServer(t)
	var body struct {
		Today     map[string]any `json:"today"`
		Yesterday map[string]any `json:"yesterday"`
	}
	rec := s.get(t, "/api/stats/today", s.admin)
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("%d %s %v", rec.Code, rec.Body, err)
	}
	if body.Today["day"] != "2026-11-19" || body.Today["users_new"] != float64(1) || body.Yesterday["day"] != "2026-11-18" || body.Yesterday["users_new"] != float64(2) {
		t.Fatalf("admin today %+v yesterday %+v", body.Today, body.Yesterday)
	}
	rec = s.get(t, "/api/stats/today", s.mod)
	body.Today, body.Yesterday = nil, nil
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("%d %s %v", rec.Code, rec.Body, err)
	}
	if _, ok := body.Today["users_new"]; ok || body.Today["reports_total"] != float64(1) || body.Yesterday["reports_total"] != float64(3) {
		t.Fatalf("moderator today %+v yesterday %+v", body.Today, body.Yesterday)
	}
}
