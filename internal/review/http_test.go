package review_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pquerna/otp/totp"

	"github.com/myronsi/messenger-moderation/internal/httpserver"
	"github.com/myronsi/messenger-moderation/internal/review"
	"github.com/myronsi/messenger-moderation/internal/staff"
	"github.com/myronsi/messenger-moderation/internal/testdb"
)

const password = "a long enough password"

type api struct {
	handler http.Handler
	mod     *pgxpool.Pool
	cookie  *http.Cookie
	staffID int64
}

func newAPI(t *testing.T, store *review.Store, reviews review.ReviewSource) api {
	t.Helper()
	pool, _ := testdb.New(t)
	svc, err := staff.NewService(pool, []byte(strings.Repeat("k", 32)), 12*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	signed := time.Now()
	created, err := svc.Create(ctx, staff.NewStaff{Username: "helper", Password: password, Role: staff.RoleModerator, ConfidentialityOn: &signed})
	if err != nil {
		t.Fatal(err)
	}
	code, err := totp.GenerateCode(created.TOTPSecret, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	session, err := svc.Login(ctx, "helper", password, code, "127.0.0.1", "test")
	if err != nil {
		t.Fatal(err)
	}
	reviewSvc := review.NewService(store, pool, reviews)
	handler := httpserver.New(httpserver.Deps{
		Staff: svc, PublicOrigin: "https://moderation.example.com",
		Register: func(mux *http.ServeMux, s *httpserver.Server) { review.Register(mux, s, reviewSvc, nil) },
	})
	return api{handler: handler, mod: pool, cookie: &http.Cookie{Name: httpserver.SessionCookie, Value: session.Token}, staffID: created.Staff.ID}
}

func (a api) get(t *testing.T, target string, signedIn bool) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	if signedIn {
		req.AddCookie(a.cookie)
	}
	rec := httptest.NewRecorder()
	a.handler.ServeHTTP(rec, req)
	return rec
}

type auditRow struct {
	StaffID    int64
	Action     string
	TargetType *string
	TargetID   *string
	Details    map[string]any
}

func (a api) auditRows(t *testing.T) []auditRow {
	t.Helper()
	rows, err := a.mod.Query(context.Background(), `SELECT staff_id, action, target_type, target_id, details FROM audit_log WHERE action NOT LIKE 'staff.%' ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []auditRow
	for rows.Next() {
		var r auditRow
		if err := rows.Scan(&r.StaffID, &r.Action, &r.TargetType, &r.TargetID, &r.Details); err != nil {
			t.Fatal(err)
		}
		out = append(out, r)
	}
	return out
}

func TestRoutesNeedASignedInStaffMember(t *testing.T) {
	_, store := standardQueue(t)
	a := newAPI(t, store, nil)
	for _, target := range []string{"/api/queue", "/api/users/4", "/api/users?q=dave"} {
		if rec := a.get(t, target, false); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s without session = %d", target, rec.Code)
		}
	}
	if rows := a.auditRows(t); len(rows) != 0 {
		t.Fatalf("audit rows without session: %+v", rows)
	}
}

func TestQueueEndpoint(t *testing.T) {
	_, store := standardQueue(t)
	a := newAPI(t, store, func(context.Context) ([]review.Review, error) {
		return []review.Review{{UserID: 5, At: time.Now()}}, nil
	})

	rec := a.get(t, "/api/queue?status=good,warned&limit=1", true)
	if rec.Code != http.StatusOK {
		t.Fatalf("queue = %d %s", rec.Code, rec.Body)
	}
	var page struct {
		Items []struct {
			User struct {
				ID       string `json:"id"`
				Username string `json:"username"`
			} `json:"user"`
			Reasons []string `json:"reasons"`
		} `json:"items"`
		NextCursor *string `json:"next_cursor"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].User.ID != "4" || page.NextCursor == nil {
		t.Fatalf("first page = %s", rec.Body)
	}
	rec = a.get(t, "/api/queue?status=good,warned&limit=1&cursor="+*page.NextCursor, true)
	if !strings.Contains(rec.Body.String(), `"username":"frank"`) || !strings.Contains(rec.Body.String(), `"next_cursor":null`) {
		t.Fatalf("second page skips the reviewed account: %s", rec.Body)
	}

	rows := a.auditRows(t)
	if len(rows) != 2 || rows[0].Action != "queue.viewed" || rows[0].StaffID != a.staffID {
		t.Fatalf("audit = %+v", rows)
	}
	if ids, _ := rows[0].Details["user_ids"].([]any); len(ids) != 1 || ids[0] != "4" {
		t.Fatalf("queue audit details = %+v", rows[0].Details)
	}

	for _, target := range []string{"/api/queue?status=banned", "/api/queue?reason=rude", "/api/queue?opened_after=yesterday", "/api/queue?limit=500", "/api/queue?cursor=@@"} {
		if rec := a.get(t, target, true); rec.Code != http.StatusBadRequest {
			t.Errorf("%s = %d %s", target, rec.Code, rec.Body)
		}
	}
}

func TestAccountEndpointLogsTheDataAccess(t *testing.T) {
	_, store := standardQueue(t)
	a := newAPI(t, store, nil)

	rec := a.get(t, "/api/users/4", true)
	if rec.Code != http.StatusOK {
		t.Fatalf("account = %d %s", rec.Code, rec.Body)
	}
	body := rec.Body.String()
	for _, want := range []string{`"id":"4"`, `"status":"warned"`, `"reason":"spam"`, `"snapshot":{"bio":null`, `"history":[]`} {
		if !strings.Contains(body, want) {
			t.Errorf("account body lacks %s: %s", want, body)
		}
	}
	if strings.Contains(body, "password") {
		t.Fatalf("account body leaks a password field: %s", body)
	}

	if rec := a.get(t, "/api/users/999", true); rec.Code != http.StatusNotFound {
		t.Fatalf("missing account = %d", rec.Code)
	}
	if rec := a.get(t, "/api/users/abc", true); rec.Code != http.StatusNotFound {
		t.Fatalf("bad id = %d", rec.Code)
	}

	rows := a.auditRows(t)
	if len(rows) != 1 {
		t.Fatalf("audit = %+v", rows)
	}
	r := rows[0]
	if r.Action != "user.viewed" || r.StaffID != a.staffID || r.TargetType == nil || *r.TargetType != "user" || r.TargetID == nil || *r.TargetID != "4" {
		t.Fatalf("audit row = %+v", r)
	}
}

func TestSearchEndpoint(t *testing.T) {
	_, store := standardQueue(t)
	a := newAPI(t, store, nil)

	rec := a.get(t, "/api/users?q=%40da", true)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"username":"dave"`) {
		t.Fatalf("search = %d %s", rec.Code, rec.Body)
	}
	if rec := a.get(t, "/api/users?q=d", true); rec.Code != http.StatusBadRequest {
		t.Fatalf("one letter = %d", rec.Code)
	}
	if rec := a.get(t, "/api/users?q=7", true); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"username":"grace"`) {
		t.Fatalf("id search = %d %s", rec.Code, rec.Body)
	}
	rows := a.auditRows(t)
	if len(rows) != 2 || rows[0].Action != "users.searched" || rows[0].Details["query"] != "da" {
		t.Fatalf("audit = %+v", rows)
	}
}

func TestWithoutTheMessengerDatabase(t *testing.T) {
	a := newAPI(t, nil, nil)
	for _, target := range []string{"/api/queue", "/api/users/4", "/api/users?q=dave"} {
		rec := a.get(t, target, true)
		if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), "messenger_unavailable") {
			t.Errorf("%s = %d %s", target, rec.Code, rec.Body)
		}
	}
}
