package content_test

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

	"github.com/myronsi/messenger-moderation/internal/appdb/appdbtest"
	"github.com/myronsi/messenger-moderation/internal/content"
	"github.com/myronsi/messenger-moderation/internal/httpserver"
	"github.com/myronsi/messenger-moderation/internal/messenger"
	"github.com/myronsi/messenger-moderation/internal/staff"
	"github.com/myronsi/messenger-moderation/internal/testdb"
)

const (
	password = "a long enough password"
	origin   = "https://moderation.example.com"
)

type removal struct {
	ChatID, MessageID int64
	Code, Text        string
}

type fakeRemover struct {
	calls []removal
	err   error
}

func (f *fakeRemover) RemoveMessage(_ context.Context, chatID, messageID int64, code, text string) (messenger.Removal, error) {
	f.calls = append(f.calls, removal{chatID, messageID, code, text})
	sender := "2"
	return messenger.Removal{ChatID: "70", MessageID: "900", SenderID: &sender, ReasonCode: code, ReasonText: text}, f.err
}

type env struct {
	t         *testing.T
	app       appdbtest.DB
	mod       *pgxpool.Pool
	remover   *fakeRemover
	handler   http.Handler
	moderator *http.Cookie
}

const messageCopy = `{"chat":{"id":"70","type":"group","title":"Shop"},"messages":[
 {"id":"899","sender_id":"1","sender_name":"Reporter","type":"text","text":"how much?","files":[],"sent_at":"2026-11-19T10:00:00Z","reported":false},
 {"id":"900","sender_id":"2","sender_name":"Spammer","type":"file","text":"cheap pills","files":["offer.pdf"],"sent_at":"2026-11-19T10:01:00Z","reported":true},
 {"id":"901","sender_id":"1","sender_name":"Reporter","type":"text","text":"no thanks","files":[],"sent_at":"2026-11-19T10:02:00Z","reported":false}]}`

const groupCopy = `{"chat":{"id":"71","type":"group","title":"Free money"},"group":{"title":"Free money","description":"Send us your card","avatar_attachment_id":null,"created_by":"2","owner_id":"2","member_count":3}}`

func newEnv(t *testing.T) *env {
	t.Helper()
	mod, _ := testdb.New(t)
	app := appdbtest.New(t)
	svc, err := staff.NewService(mod, []byte("0123456789abcdef0123456789abcdef"), 12*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	e := &env{t: t, app: app, mod: mod, remover: &fakeRemover{}}
	e.moderator = e.login(svc, "helper")
	contentSvc := content.NewService(mod, app.Read, e.remover, nil)
	e.handler = httpserver.New(httpserver.Deps{
		Staff: svc, PublicOrigin: origin,
		Register: func(mux *http.ServeMux, s *httpserver.Server) { content.Register(mux, s, contentSvc, nil) },
	})
	e.exec(`INSERT INTO users (id, username, display_name) VALUES (1, 'reporter', 'Reporter'), (2, 'spammer', 'Spammer'), (3, 'second', 'Second')`)
	e.exec(`INSERT INTO content_reports (id, reporter_id, kind, chat_id, message_id, target_id, reason, note, content, created_at) VALUES
		(10, 1, 'message', 70, 900, 2, 'illegal', 'Sells prescription drugs.', $1, now() - interval '2 hours'),
		(11, 3, 'message', 70, 900, 2, 'spam', NULL, $1, now() - interval '1 hour'),
		(12, 1, 'group', 71, 0, 2, 'scam', NULL, $2, now() - interval '3 hours')`, messageCopy, groupCopy)
	e.exec(`INSERT INTO content_reports (id, reporter_id, kind, chat_id, message_id, target_id, reason, content, withdrawn_at) VALUES
		(13, 3, 'message', 72, 950, 2, 'spam', $1, now())`, messageCopy)
	return e
}

func (e *env) login(svc *staff.Service, name string) *http.Cookie {
	e.t.Helper()
	ctx := context.Background()
	signed := time.Now()
	created, err := svc.Create(ctx, staff.NewStaff{Username: name, Password: password, Role: staff.RoleModerator, ConfidentialityOn: &signed})
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

func (e *env) audit(action string) []map[string]any {
	e.t.Helper()
	rows, err := e.mod.Query(context.Background(), `SELECT target_id, details FROM audit_log WHERE action = $1 ORDER BY id`, action)
	if err != nil {
		e.t.Fatal(err)
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var target *string
		var raw []byte
		if err := rows.Scan(&target, &raw); err != nil {
			e.t.Fatal(err)
		}
		d := map[string]any{}
		if err := json.Unmarshal(raw, &d); err != nil {
			e.t.Fatal(err)
		}
		if target != nil {
			d["_target"] = *target
		}
		out = append(out, d)
	}
	return out
}

type listBody struct {
	Reports []struct {
		ID       string `json:"id"`
		Kind     string `json:"kind"`
		Reason   string `json:"reason"`
		Excerpt  string `json:"excerpt"`
		Reports  int    `json:"reports_on_item"`
		Decision *struct {
			Action string `json:"action"`
		} `json:"decision"`
		Target *struct {
			Username string `json:"username"`
		} `json:"target"`
	} `json:"reports"`
}

func (e *env) list(state string) listBody {
	e.t.Helper()
	rec := e.do(e.moderator, http.MethodGet, "/api/content-reports?state="+state, "")
	if rec.Code != http.StatusOK {
		e.t.Fatalf("list %s: %d %s", state, rec.Code, rec.Body)
	}
	var out listBody
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		e.t.Fatal(err)
	}
	return out
}

func TestListShowsOneRowPerReportedItem(t *testing.T) {
	e := newEnv(t)
	if rec := e.do(nil, http.MethodGet, "/api/content-reports", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("without a session: %d", rec.Code)
	}
	open := e.list("open")
	if len(open.Reports) != 2 {
		t.Fatalf("open items: %+v", open.Reports)
	}
	first, second := open.Reports[0], open.Reports[1]
	if first.ID != "11" || first.Reports != 2 || first.Excerpt != "cheap pills" || first.Target == nil || first.Target.Username != "spammer" {
		t.Fatalf("newest message item: %+v", first)
	}
	if second.ID != "12" || second.Kind != "group" || second.Excerpt != "Free money" {
		t.Fatalf("group item: %+v", second)
	}
	if len(e.list("decided").Reports) != 0 {
		t.Fatal("nothing is decided yet")
	}
	if got := e.audit("content_reports.viewed"); len(got) != 2 {
		t.Fatalf("data access log: %v", got)
	}
}

func TestReportPageShowsTheCopyAndTheOtherReports(t *testing.T) {
	e := newEnv(t)
	rec := e.do(e.moderator, http.MethodGet, "/api/content-reports/10", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	var d struct {
		Reason   string `json:"reason"`
		Note     string `json:"note"`
		Reporter struct {
			Username string `json:"username"`
		} `json:"reporter"`
		Copy struct {
			Messages []struct {
				ID       string `json:"id"`
				Reported bool   `json:"reported"`
			} `json:"messages"`
		} `json:"copy"`
		Others []struct {
			ID     string `json:"id"`
			Reason string `json:"reason"`
		} `json:"other_reports"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &d); err != nil {
		t.Fatal(err)
	}
	if d.Reason != "illegal" || d.Note != "Sells prescription drugs." || d.Reporter.Username != "reporter" {
		t.Fatalf("report: %+v", d)
	}
	if len(d.Copy.Messages) != 3 || !d.Copy.Messages[1].Reported || d.Copy.Messages[1].ID != "900" {
		t.Fatalf("copy: %+v", d.Copy)
	}
	if len(d.Others) != 1 || d.Others[0].ID != "11" || d.Others[0].Reason != "spam" {
		t.Fatalf("other reports: %+v", d.Others)
	}
	if got := e.audit("content_report.viewed"); len(got) != 1 || got[0]["_target"] != "10" {
		t.Fatalf("data access log: %v", got)
	}
	if rec := e.do(e.moderator, http.MethodGet, "/api/content-reports/999", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown report: %d", rec.Code)
	}
}

func TestRemoveMessageCallsMessengerAndRecordsTheDecision(t *testing.T) {
	e := newEnv(t)
	body := `{"reason_code":"illegal","reason_text":"  Selling prescription drugs is not allowed.  "}`
	if rec := e.do(e.moderator, http.MethodPost, "/api/content-reports/10/remove", `{"reason_code":"rude","reason_text":"x"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown rule: %d", rec.Code)
	}
	if rec := e.do(e.moderator, http.MethodPost, "/api/content-reports/10/remove", `{"reason_code":"spam","reason_text":"  "}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("empty text: %d", rec.Code)
	}
	if rec := e.do(e.moderator, http.MethodPost, "/api/content-reports/12/remove", `{"reason_code":"scam","reason_text":"x"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("group report: %d", rec.Code)
	}
	if len(e.remover.calls) != 0 {
		t.Fatalf("invalid requests reached Messenger: %v", e.remover.calls)
	}
	rec := e.do(e.moderator, http.MethodPost, "/api/content-reports/10/remove", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("remove: %d %s", rec.Code, rec.Body)
	}
	if len(e.remover.calls) != 1 || e.remover.calls[0] != (removal{70, 900, "illegal", "Selling prescription drugs is not allowed."}) {
		t.Fatalf("messenger calls: %+v", e.remover.calls)
	}
	logged := e.audit("message.removed")
	if len(logged) != 1 || logged[0]["_target"] != "70:900" || logged[0]["report_id"] != "10" || logged[0]["sender_id"] != "2" {
		t.Fatalf("audit: %v", logged)
	}
	if open := e.list("open"); len(open.Reports) != 1 || open.Reports[0].ID != "12" {
		t.Fatalf("both reports on the removed message left the open list: %+v", open.Reports)
	}
	decided := e.list("decided")
	if len(decided.Reports) != 1 || decided.Reports[0].Decision == nil || decided.Reports[0].Decision.Action != content.ActionRemoved {
		t.Fatalf("decided: %+v", decided.Reports)
	}
}

func TestRemoveMessageReportsMessengerFailures(t *testing.T) {
	e := newEnv(t)
	e.remover.err = messenger.ErrNotFound
	if rec := e.do(e.moderator, http.MethodPost, "/api/content-reports/10/remove", `{"reason_code":"spam","reason_text":"Ads"}`); rec.Code != http.StatusNotFound {
		t.Fatalf("message gone: %d %s", rec.Code, rec.Body)
	}
	e.remover.err = messenger.ErrNotConfigured
	if rec := e.do(e.moderator, http.MethodPost, "/api/content-reports/10/remove", `{"reason_code":"spam","reason_text":"Ads"}`); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("not connected: %d %s", rec.Code, rec.Body)
	}
	if got := e.audit("message.removed"); len(got) != 0 {
		t.Fatalf("a failed removal was recorded: %v", got)
	}
	if open := e.list("open"); len(open.Reports) != 2 {
		t.Fatalf("a failed removal closed the item: %+v", open.Reports)
	}
}

func TestDismissClosesTheItemWithoutCallingMessenger(t *testing.T) {
	e := newEnv(t)
	if rec := e.do(e.moderator, http.MethodPost, "/api/content-reports/12/dismiss", `{"note":"Satire group, no scam."}`); rec.Code != http.StatusNoContent {
		t.Fatalf("dismiss: %d %s", rec.Code, rec.Body)
	}
	if len(e.remover.calls) != 0 {
		t.Fatalf("messenger calls: %v", e.remover.calls)
	}
	if got := e.audit("content_report.dismissed"); len(got) != 1 || got[0]["note"] != "Satire group, no scam." || got[0]["kind"] != "group" {
		t.Fatalf("audit: %v", got)
	}
	decided := e.list("decided")
	if len(decided.Reports) != 1 || decided.Reports[0].Decision.Action != content.ActionNone {
		t.Fatalf("decided: %+v", decided.Reports)
	}
	if rec := e.do(e.moderator, http.MethodPost, "/api/content-reports/10/remove", `{"reason_code":"spam","reason_text":"Ads after all"}`); rec.Code != http.StatusOK {
		t.Fatalf("removing after a dismissal on another item: %d", rec.Code)
	}
}
