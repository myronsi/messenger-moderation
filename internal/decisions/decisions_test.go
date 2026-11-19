package decisions_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pquerna/otp/totp"

	"github.com/myronsi/messenger-moderation/internal/appdb/appdbtest"
	"github.com/myronsi/messenger-moderation/internal/cases"
	"github.com/myronsi/messenger-moderation/internal/decisions"
	"github.com/myronsi/messenger-moderation/internal/httpserver"
	"github.com/myronsi/messenger-moderation/internal/messenger"
	"github.com/myronsi/messenger-moderation/internal/staff"
	"github.com/myronsi/messenger-moderation/internal/testdb"
)

const (
	password = "a long enough password"
	origin   = "https://moderation.example.com"
	imageID  = "0b7f9a3e-1c2d-4e5f-8a9b-0c1d2e3f4a5b"
)

type call struct {
	Method string
	UserID int64
	Status messenger.StatusRequest
	IDs    []int64
	Kind   string
	PackID int64
	Hidden bool
}

type fakeMessenger struct {
	calls []call
	err   error
}

func (f *fakeMessenger) SetStatus(_ context.Context, userID int64, req messenger.StatusRequest) (messenger.Change, error) {
	f.calls = append(f.calls, call{Method: "status", UserID: userID, Status: req})
	return messenger.Change{Before: messenger.StatusView{Status: "restricted"}, After: messenger.StatusView{Status: req.Status, DecidedBy: "staff"}}, f.err
}

func (f *fakeMessenger) DismissReports(_ context.Context, userID int64, ids []int64) (messenger.Dismissal, error) {
	f.calls = append(f.calls, call{Method: "dismiss", UserID: userID, IDs: ids})
	return messenger.Dismissal{Dismissed: 2, Before: messenger.StatusView{Status: "limited"}, After: messenger.StatusView{Status: "warned"}}, f.err
}

func (f *fakeMessenger) SetPackHidden(_ context.Context, kind string, id int64, hidden bool) (bool, error) {
	f.calls = append(f.calls, call{Method: "pack", Kind: kind, PackID: id, Hidden: hidden})
	return true, f.err
}

func (f *fakeMessenger) DeleteAccount(_ context.Context, userID int64) error {
	f.calls = append(f.calls, call{Method: "remove", UserID: userID})
	return f.err
}

func (f *fakeMessenger) PackImage(_ context.Context, id string) (*http.Response, error) {
	f.calls = append(f.calls, call{Method: "image"})
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"image/webp"}}, Body: io.NopCloser(strings.NewReader("webp:" + id))}, f.err
}

type env struct {
	t         *testing.T
	app       appdbtest.DB
	mod       *pgxpool.Pool
	messenger *fakeMessenger
	cases     *cases.Store
	handler   http.Handler
	moderator *http.Cookie
	admin     *http.Cookie
}

func newEnv(t *testing.T) *env {
	t.Helper()
	mod, _ := testdb.New(t)
	app := appdbtest.New(t)
	svc, err := staff.NewService(mod, []byte(strings.Repeat("k", 32)), 12*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	e := &env{t: t, app: app, mod: mod, messenger: &fakeMessenger{}, cases: cases.NewStore(mod, app.Read, nil)}
	e.moderator = e.login(svc, "helper", staff.RoleModerator)
	e.admin = e.login(svc, "boss", staff.RoleAdmin)
	decisionSvc := decisions.NewService(mod, app.Read, e.messenger, e.cases, nil)
	e.handler = httpserver.New(httpserver.Deps{
		Staff: svc, PublicOrigin: origin,
		Register: func(mux *http.ServeMux, s *httpserver.Server) { decisions.Register(mux, s, decisionSvc, nil) },
	})
	e.exec(`INSERT INTO users (id, username, display_name) VALUES (1, 'reporter', 'Reporter'), (2, 'spammer', 'Spammer'), (3, 'owner', 'Owner')`)
	return e
}

func (e *env) login(svc *staff.Service, name string, role staff.Role) *http.Cookie {
	e.t.Helper()
	ctx := context.Background()
	signed := time.Now()
	created, err := svc.Create(ctx, staff.NewStaff{Username: name, Password: password, Role: role, ConfidentialityOn: &signed})
	if err != nil {
		e.t.Fatal(err)
	}
	code, err := totp.GenerateCode(created.TOTPSecret, time.Now())
	if err != nil {
		e.t.Fatal(err)
	}
	session, err := svc.Login(ctx, name, password, code, "127.0.0.1", "test")
	if err != nil {
		e.t.Fatal(err)
	}
	return &http.Cookie{Name: httpserver.SessionCookie, Value: session.Token}
}

func (e *env) exec(sql string, args ...any) {
	e.t.Helper()
	if _, err := e.app.Write.Exec(context.Background(), sql, args...); err != nil {
		e.t.Fatalf("%s: %v", sql, err)
	}
}

func (e *env) do(cookie *http.Cookie, method, target, body string) *httptest.ResponseRecorder {
	e.t.Helper()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("Origin", origin)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	e.handler.ServeHTTP(rec, req)
	return rec
}

func (e *env) scan() int {
	e.t.Helper()
	n, err := e.cases.Scan(context.Background())
	if err != nil {
		e.t.Fatal(err)
	}
	return n
}

func (e *env) caseStates() map[int64]string {
	e.t.Helper()
	rows, err := e.mod.Query(context.Background(), `SELECT id, state FROM cases`)
	if err != nil {
		e.t.Fatal(err)
	}
	defer rows.Close()
	out := map[int64]string{}
	for rows.Next() {
		var id int64
		var state string
		if err := rows.Scan(&id, &state); err != nil {
			e.t.Fatal(err)
		}
		out[id] = state
	}
	return out
}

func (e *env) audit(action string) []map[string]any {
	e.t.Helper()
	rows, err := e.mod.Query(context.Background(), `SELECT details FROM audit_log WHERE action = $1 ORDER BY id`, action)
	if err != nil {
		e.t.Fatal(err)
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var d map[string]any
		if err := rows.Scan(&d); err != nil {
			e.t.Fatal(err)
		}
		out = append(out, d)
	}
	return out
}

func decode(t *testing.T, rec *httptest.ResponseRecorder, v any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), v); err != nil {
		t.Fatalf("decode %s: %v", rec.Body, err)
	}
}

const reason = `"reason_code":"spam","reason_text":"Sent the same link to many people."`

func TestWatcherOpensCasesForHeavyAutomaticLevelsAndAutoHiddenPacks(t *testing.T) {
	e := newEnv(t)
	at := time.Now().Add(-time.Hour).UTC().Truncate(time.Second)
	e.exec(`INSERT INTO user_moderation_events (user_id, from_status, to_status, score, cause, created_at) VALUES
(2, 'limited', 'restricted', 10, 'reports', $1),
(1, 'good', 'warned', 3, 'reports', $1),
(1, 'warned', 'suspended', 0, 'manual', $1),
(1, 'suspended', 'restricted', 0, 'appeal', $1),
(3, 'restricted', 'suspended', 16, 'reports', $2)`, at, at.Add(-40*24*time.Hour))
	e.exec(`INSERT INTO sticker_packs (id, owner_id, title, hidden_at, hidden_by) VALUES (7, 3, 'Auto', $1, 'reports'), (8, 3, 'By staff', $1, 'staff'), (9, 3, 'Visible', NULL, NULL)`, at)
	e.exec(`INSERT INTO emoji_packs (id, owner_id, title, slug, hidden_at, hidden_by) VALUES (4, 3, 'Emoji', 'EMOJIPACK1', $1, 'reports')`, at)

	if n := e.scan(); n != 3 {
		t.Fatalf("opened %d cases, want 3", n)
	}
	if n := e.scan(); n != 0 {
		t.Fatalf("a second scan opened %d more", n)
	}
	rec := e.do(e.moderator, http.MethodGet, "/api/cases", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("cases %d %s", rec.Code, rec.Body)
	}
	var body struct {
		Cases []decisions.CaseView `json:"cases"`
	}
	decode(t, rec, &body)
	if len(body.Cases) != 3 {
		t.Fatalf("cases %+v", body.Cases)
	}
	account := body.Cases[0]
	if account.Kind != cases.KindAccountStatus || *account.UserID != 2 || *account.ToStatus != "restricted" || account.Subject.Username != "spammer" {
		t.Fatalf("account case %+v subject %+v", account.Case, account.Subject)
	}
	if !account.Deadline.Equal(at.Add(cases.Deadline)) || account.State != cases.StateOpen {
		t.Fatalf("deadline %v state %s", account.Deadline, account.State)
	}
	kinds := map[string]bool{}
	for _, c := range body.Cases[1:] {
		kinds[*c.PackKind] = true
		if c.Subject == nil || c.Subject.Title == "" || !*c.Subject.Hidden {
			t.Fatalf("pack subject %+v", c.Subject)
		}
	}
	if !kinds["sticker_pack"] || !kinds["emoji_pack"] {
		t.Fatalf("pack cases %v", kinds)
	}
	if rows := e.audit("cases.viewed"); len(rows) != 1 || rows[0]["state"] != "open" {
		t.Fatalf("data access log %v", rows)
	}

	e.exec(`INSERT INTO user_moderation_events (user_id, from_status, to_status, score, cause, created_at) VALUES (2, 'restricted', 'suspended', 15, 'reports', now())`)
	if n := e.scan(); n != 1 {
		t.Fatalf("a higher level opened %d", n)
	}
	superseded := 0
	for _, s := range e.caseStates() {
		if s == cases.StateSuperseded {
			superseded++
		}
	}
	if superseded != 1 {
		t.Fatalf("the older case of the account is superseded: %v", e.caseStates())
	}
}

func TestSetStatusClosesCasesAndAudits(t *testing.T) {
	e := newEnv(t)
	e.exec(`INSERT INTO user_moderation_events (user_id, from_status, to_status, score, cause, created_at) VALUES (2, 'limited', 'suspended', 15, 'reports', now())`)
	e.scan()

	rec := e.do(e.moderator, http.MethodPost, "/api/users/2/status", `{"status":"restricted",`+reason+`}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d %s", rec.Code, rec.Body)
	}
	got := e.messenger.calls[0]
	if got.Method != "status" || got.UserID != 2 || got.Status.Status != "restricted" || got.Status.ReasonCode != "spam" || !strings.HasPrefix(got.Status.StaffRef, "staff:") {
		t.Fatalf("messenger call %+v", got)
	}
	for _, s := range e.caseStates() {
		if s != cases.StateReversed {
			t.Fatalf("a lower level reverses the case: %v", e.caseStates())
		}
	}
	rows := e.audit("user.status_set")
	if len(rows) != 1 || rows[0]["reason_text"] != "Sent the same link to many people." || rows[0]["before"].(map[string]any)["status"] != "restricted" {
		t.Fatalf("audit %v", rows)
	}
	if closed, ok := rows[0]["closed_cases"].([]any); !ok || len(closed) != 1 {
		t.Fatalf("closed cases in the audit %v", rows[0])
	}

	for name, body := range map[string]string{
		"no reason":      `{"status":"warned"}`,
		"unknown rule":   `{"status":"warned","reason_code":"rude","reason_text":"x"}`,
		"blank text":     `{"status":"warned","reason_code":"spam","reason_text":"   "}`,
		"unknown status": `{"status":"banned",` + reason + `}`,
		"unknown field":  `{"status":"warned","note":"x",` + reason + `}`,
	} {
		if rec := e.do(e.moderator, http.MethodPost, "/api/users/2/status", body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: %d %s", name, rec.Code, rec.Body)
		}
	}
	if len(e.messenger.calls) != 1 {
		t.Fatalf("invalid requests reached Messenger: %+v", e.messenger.calls)
	}
	if rec := e.do(nil, http.MethodPost, "/api/users/2/status", `{"status":"good",`+reason+`}`); rec.Code != http.StatusUnauthorized {
		t.Fatalf("without a session %d", rec.Code)
	}
	e.messenger.err = messenger.ErrNotFound
	if rec := e.do(e.moderator, http.MethodPost, "/api/users/99/status", `{"status":"good",`+reason+`}`); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown account %d", rec.Code)
	}
	e.messenger.err = &messenger.FieldError{Field: "reason_text", Message: "1 to 1000 characters"}
	if rec := e.do(e.moderator, http.MethodPost, "/api/users/2/status", `{"status":"good",`+reason+`}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("refused by Messenger %d", rec.Code)
	}
	if rows := e.audit("user.status_set"); len(rows) != 1 {
		t.Fatalf("failed decisions are not recorded: %v", rows)
	}
}

func TestConfirmAndReverseCases(t *testing.T) {
	e := newEnv(t)
	e.exec(`INSERT INTO user_moderation_events (user_id, from_status, to_status, score, cause, created_at) VALUES (2, 'limited', 'restricted', 10, 'reports', now())`)
	e.exec(`INSERT INTO sticker_packs (id, owner_id, title, hidden_at, hidden_by) VALUES (7, 3, 'Auto', now(), 'reports')`)
	e.scan()
	var accountCase, packCase int64
	if err := e.mod.QueryRow(context.Background(), `SELECT id FROM cases WHERE kind = 'account_status'`).Scan(&accountCase); err != nil {
		t.Fatal(err)
	}
	if err := e.mod.QueryRow(context.Background(), `SELECT id FROM cases WHERE kind = 'pack_hidden'`).Scan(&packCase); err != nil {
		t.Fatal(err)
	}
	path := func(id int64, action string) string { return "/api/cases/" + itoa(id) + "/" + action }

	if rec := e.do(e.moderator, http.MethodPost, path(accountCase, "reverse"), `{"status":"restricted",`+reason+`}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("a reversal must lower the level: %d %s", rec.Code, rec.Body)
	}
	if rec := e.do(e.moderator, http.MethodPost, path(accountCase, "confirm"), `{`+reason+`}`); rec.Code != http.StatusNoContent {
		t.Fatalf("confirm %d %s", rec.Code, rec.Body)
	}
	if c := e.messenger.calls[0]; c.Method != "status" || c.Status.Status != "restricted" {
		t.Fatalf("confirming keeps the level: %+v", c)
	}
	if rec := e.do(e.moderator, http.MethodPost, path(accountCase, "confirm"), `{`+reason+`}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("a closed case %d", rec.Code)
	}
	if rec := e.do(e.moderator, http.MethodPost, path(packCase, "reverse"), `{`+reason+`}`); rec.Code != http.StatusNoContent {
		t.Fatalf("reverse pack %d %s", rec.Code, rec.Body)
	}
	if c := e.messenger.calls[1]; c.Method != "pack" || c.Kind != "sticker_pack" || c.PackID != 7 || c.Hidden {
		t.Fatalf("reversing a pack case unhides it: %+v", c)
	}
	states := e.caseStates()
	if states[accountCase] != cases.StateConfirmed || states[packCase] != cases.StateReversed {
		t.Fatalf("states %v", states)
	}
	rec := e.do(e.moderator, http.MethodGet, "/api/cases?state=decided", "")
	var body struct {
		Cases []decisions.CaseView `json:"cases"`
	}
	decode(t, rec, &body)
	if len(body.Cases) != 2 || body.Cases[0].DecidedBy == nil || *body.Cases[0].DecidedBy != "helper" || *body.Cases[0].ReasonCode != "spam" {
		t.Fatalf("decided cases %+v", body.Cases)
	}
	if rec := e.do(e.moderator, http.MethodPost, path(9999, "confirm"), `{`+reason+`}`); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown case %d", rec.Code)
	}
}

func TestDismissReports(t *testing.T) {
	e := newEnv(t)
	rec := e.do(e.moderator, http.MethodPost, "/api/users/2/reports/dismiss", `{"reporter_ids":["1"],`+reason+`}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("dismiss %d %s", rec.Code, rec.Body)
	}
	if c := e.messenger.calls[0]; c.Method != "dismiss" || c.UserID != 2 || len(c.IDs) != 1 || c.IDs[0] != 1 {
		t.Fatalf("messenger call %+v", c)
	}
	if rec := e.do(e.moderator, http.MethodPost, "/api/users/2/reports/dismiss", `{`+reason+`}`); rec.Code != http.StatusOK || e.messenger.calls[1].IDs != nil {
		t.Fatalf("dismiss all %d %+v", rec.Code, e.messenger.calls)
	}
	if rec := e.do(e.moderator, http.MethodPost, "/api/users/2/reports/dismiss", `{"reporter_ids":[],`+reason+`}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("empty list %d", rec.Code)
	}
	if rows := e.audit("user.reports_dismissed"); len(rows) != 2 || rows[0]["dismissed"] != float64(2) {
		t.Fatalf("audit %v", rows)
	}
}

func TestOnlyAdminsDeleteAccounts(t *testing.T) {
	e := newEnv(t)
	if rec := e.do(e.moderator, http.MethodDelete, "/api/users/2", `{`+reason+`}`); rec.Code != http.StatusForbidden {
		t.Fatalf("moderator %d", rec.Code)
	}
	if len(e.messenger.calls) != 0 {
		t.Fatal("a moderator's request reached Messenger")
	}
	if rec := e.do(e.admin, http.MethodDelete, "/api/users/2", `{`+reason+`}`); rec.Code != http.StatusNoContent {
		t.Fatalf("admin %d %s", rec.Code, rec.Body)
	}
	if c := e.messenger.calls[0]; c.Method != "remove" || c.UserID != 2 {
		t.Fatalf("messenger call %+v", c)
	}
	if rows := e.audit("user.deleted"); len(rows) != 1 {
		t.Fatalf("audit %v", rows)
	}
}

func TestPackPage(t *testing.T) {
	e := newEnv(t)
	e.exec(`INSERT INTO sticker_packs (id, owner_id, title, slug, hidden_at, hidden_by) VALUES (7, 3, 'Cats', 'CATSPACK1', now(), 'reports')`)
	e.exec(`INSERT INTO stickers (pack_id, attachment_id, emoji, position) VALUES (7, $1, '😺', 0), (7, gen_random_uuid(), '😾', 1)`, imageID)
	e.exec(`INSERT INTO stickers (pack_id, attachment_id, emoji, position, deleted_at) VALUES (7, gen_random_uuid(), '🙀', 2, now())`)
	e.exec(`INSERT INTO sticker_pack_reports (pack_id, reporter_id, reason) VALUES (7, 1, 'violence')`)
	e.exec(`INSERT INTO emoji_packs (id, owner_id, title, slug) VALUES (4, 3, 'Faces', 'FACESPACK1')`)
	e.exec(`INSERT INTO custom_emoji (pack_id, attachment_id, shortcode, position) VALUES (4, gen_random_uuid(), 'smile', 0)`)
	e.scan()

	rec := e.do(e.moderator, http.MethodGet, "/api/packs/sticker_pack/7", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("pack %d %s", rec.Code, rec.Body)
	}
	var p decisions.Pack
	decode(t, rec, &p)
	if p.Title != "Cats" || p.Owner == nil || p.Owner.Username != "owner" || len(p.Items) != 2 || p.Items[0].Label != "😺" || p.Items[0].AttachmentID != imageID {
		t.Fatalf("pack %+v", p)
	}
	if len(p.Reports) != 1 || p.Reports[0].Reason != "violence" || p.Reports[0].ReporterUsername != "reporter" || len(p.Cases) != 1 || p.HiddenBy == nil || *p.HiddenBy != "reports" {
		t.Fatalf("reports %+v cases %+v", p.Reports, p.Cases)
	}
	rec = e.do(e.moderator, http.MethodGet, "/api/packs/emoji_pack/4", "")
	decode(t, rec, &p)
	if rec.Code != http.StatusOK || len(p.Items) != 1 || p.Items[0].Label != "smile" {
		t.Fatalf("emoji pack %d %+v", rec.Code, p)
	}
	if rec := e.do(e.moderator, http.MethodGet, "/api/packs/sticker_pack/99", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown pack %d", rec.Code)
	}
	if rec := e.do(e.moderator, http.MethodGet, "/api/packs/chat/7", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown kind %d", rec.Code)
	}

	rec = e.do(e.moderator, http.MethodGet, "/api/packs/sticker_pack/7/images/"+imageID, "")
	if rec.Code != http.StatusOK || rec.Body.String() != "webp:"+imageID || rec.Header().Get("Content-Type") != "image/webp" {
		t.Fatalf("image %d %q", rec.Code, rec.Body)
	}
	if rec := e.do(e.moderator, http.MethodGet, "/api/packs/emoji_pack/4/images/"+imageID, ""); rec.Code != http.StatusNotFound {
		t.Fatalf("an image of another pack %d", rec.Code)
	}
	if rec := e.do(nil, http.MethodGet, "/api/packs/sticker_pack/7/images/"+imageID, ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("image without a session %d", rec.Code)
	}

	rec = e.do(e.moderator, http.MethodGet, "/api/packs", "")
	var list struct {
		Packs []decisions.ReportedPack `json:"packs"`
	}
	decode(t, rec, &list)
	if len(list.Packs) != 1 || list.Packs[0].ID != 7 || list.Packs[0].Reports != 1 {
		t.Fatalf("reported packs %+v", list.Packs)
	}

	if rec := e.do(e.moderator, http.MethodPost, "/api/packs/sticker_pack/7/hide", `{`+reason+`}`); rec.Code != http.StatusOK {
		t.Fatalf("hide %d %s", rec.Code, rec.Body)
	}
	for _, s := range e.caseStates() {
		if s != cases.StateConfirmed {
			t.Fatalf("hiding confirms the pack case: %v", e.caseStates())
		}
	}
	if rows := e.audit("pack.viewed"); len(rows) != 2 {
		t.Fatalf("pack views are logged: %v", rows)
	}
	if rows := e.audit("pack.hidden"); len(rows) != 1 {
		t.Fatalf("audit %v", rows)
	}
}

func TestReviewsFeedTheQueue(t *testing.T) {
	e := newEnv(t)
	svc := decisions.NewService(e.mod, e.app.Read, e.messenger, e.cases, nil)
	if rec := e.do(e.moderator, http.MethodPost, "/api/users/2/status", `{"status":"warned",`+reason+`}`); rec.Code != http.StatusOK {
		t.Fatal(rec.Body)
	}
	reviews, err := svc.Reviews(context.Background())
	if err != nil || len(reviews) != 1 || reviews[0].UserID != 2 || time.Since(reviews[0].At) > time.Minute {
		t.Fatalf("reviews %+v (%v)", reviews, err)
	}
}

func TestMessengerNotConfigured(t *testing.T) {
	e := newEnv(t)
	e.messenger.err = messenger.ErrNotConfigured
	if rec := e.do(e.moderator, http.MethodPost, "/api/users/2/status", `{"status":"good",`+reason+`}`); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
