package decisions_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/myronsi/messenger-moderation/internal/cases"
	"github.com/myronsi/messenger-moderation/internal/decisions"
	"github.com/myronsi/messenger-moderation/internal/notices"
	"github.com/myronsi/messenger-moderation/internal/staff"
)

type mail struct{ To, Subject, Body string }

type fakeMailer struct {
	mu   sync.Mutex
	sent []mail
	err  error
}

func (m *fakeMailer) Send(_ context.Context, to, subject, body string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return m.err
	}
	m.sent = append(m.sent, mail{to, subject, body})
	return nil
}

func (e *env) listCases(query string) []decisions.CaseView {
	e.t.Helper()
	rec := e.do(e.moderator, http.MethodGet, "/api/cases"+query, "")
	if rec.Code != http.StatusOK {
		e.t.Fatalf("cases%s: %d %s", query, rec.Code, rec.Body)
	}
	var body struct {
		Cases []decisions.CaseView `json:"cases"`
	}
	decode(e.t, rec, &body)
	return body.Cases
}

func (e *env) caseID(kind string) int64 {
	e.t.Helper()
	var id int64
	if err := e.mod.QueryRow(context.Background(), `SELECT id FROM cases WHERE kind = $1 ORDER BY id DESC LIMIT 1`, kind).Scan(&id); err != nil {
		e.t.Fatal(err)
	}
	return id
}

func TestReviewRequestsBecomeCases(t *testing.T) {
	e := newEnv(t)
	at := time.Now().Add(-2 * time.Hour).UTC().Truncate(time.Second)
	e.exec(`INSERT INTO user_moderation (user_id, status) VALUES (2, 'restricted')`)
	e.exec(`INSERT INTO account_review_requests (user_id, status, text, created_at) VALUES (2, 'restricted', 'I only sent one link.', $1)`, at)
	e.exec(`INSERT INTO account_review_requests (user_id, status, text, created_at, closed_at, close_reason) VALUES (3, 'warned', 'old', $1, $1, 'decided')`, at)
	if n := e.scan(); n != 1 {
		t.Fatalf("opened %d cases, want 1 for the open request", n)
	}
	if n := e.scan(); n != 0 {
		t.Fatalf("a second scan opened %d cases", n)
	}
	list := e.listCases("?kind=review_request")
	if len(list) != 1 {
		t.Fatalf("review request cases %+v", list)
	}
	c := list[0]
	if *c.UserID != 2 || *c.ToStatus != "restricted" || c.RequestText == nil || *c.RequestText != "I only sent one link." || !c.Deadline.Equal(at.Add(cases.Deadline)) {
		t.Fatalf("case %+v", c)
	}
	if c.Subject == nil || c.Subject.Username != "spammer" {
		t.Fatalf("subject %+v", c.Subject)
	}
	if other := e.listCases("?kind=pack_hidden"); len(other) != 0 {
		t.Fatalf("kind filter %+v", other)
	}
	if rec := e.do(e.moderator, http.MethodGet, "/api/cases?kind=nope", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown kind %d", rec.Code)
	}
}

func TestReviewRequestClosesWhenTheStatusIsGoodAgain(t *testing.T) {
	e := newEnv(t)
	e.exec(`INSERT INTO account_review_requests (user_id, status, text) VALUES (2, 'warned', 'why')`)
	e.scan()
	id := e.caseID(cases.KindReviewRequest)
	e.exec(`UPDATE account_review_requests SET closed_at = now(), close_reason = 'status_good'`)
	e.scan()
	if state := e.caseStates()[id]; state != cases.StateSuperseded {
		t.Fatalf("state %s, want superseded", state)
	}
}

func TestDecideReviewRequest(t *testing.T) {
	e := newEnv(t)
	e.exec(`INSERT INTO user_moderation (user_id, status) VALUES (2, 'limited')`)
	e.exec(`INSERT INTO account_review_requests (user_id, status, text) VALUES (2, 'restricted', 'please')`)
	e.scan()
	id := e.caseID(cases.KindReviewRequest)
	path := func(action string) string { return "/api/cases/" + itoa(id) + "/" + action }
	if rec := e.do(e.moderator, http.MethodPost, path("reverse"), `{"status":"limited",`+reason+`}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("a reversal must go below the current status: %d %s", rec.Code, rec.Body)
	}
	if rec := e.do(e.moderator, http.MethodPost, path("confirm"), `{`+reason+`}`); rec.Code != http.StatusNoContent {
		t.Fatalf("confirm %d %s", rec.Code, rec.Body)
	}
	if c := e.messenger.calls[0]; c.Method != "status" || c.UserID != 2 || c.Status.Status != "limited" || c.Status.ReasonCode != "spam" {
		t.Fatalf("confirming keeps the current status as a decision by a person: %+v", c)
	}
	if state := e.caseStates()[id]; state == cases.StateOpen {
		t.Fatal("the case stays open")
	}

	e.exec(`INSERT INTO account_review_requests (user_id, status, text, created_at) VALUES (1, 'warned', 'second', now())`)
	e.exec(`INSERT INTO user_moderation (user_id, status) VALUES (1, 'warned')`)
	e.scan()
	second := e.caseID(cases.KindReviewRequest)
	if rec := e.do(e.moderator, http.MethodPost, "/api/users/1/status", `{"status":"good",`+reason+`}`); rec.Code != http.StatusOK {
		t.Fatalf("decision from the account page %d %s", rec.Code, rec.Body)
	}
	if state := e.caseStates()[second]; state != cases.StateReversed {
		t.Fatalf("a decision on the account closes its review request case: %s", state)
	}
}

const noticeBody = `{"notifier_name":"Jane Doe","notifier_email":"jane@example.com","content":"A message calling for violence",` +
	`"location":"Group 'Town news', message of 18 November","explanation":"It incites violence against a named person.",` +
	`"received_at":"RECEIVED","user_id":"2"}`

func (e *env) addNotice(body string) int64 {
	e.t.Helper()
	body = strings.Replace(body, "RECEIVED", time.Now().Add(-3*time.Hour).UTC().Format(time.RFC3339), 1)
	rec := e.do(e.moderator, http.MethodPost, "/api/notices", body)
	if rec.Code != http.StatusCreated {
		e.t.Fatalf("add notice %d %s", rec.Code, rec.Body)
	}
	var out struct {
		ID string `json:"id"`
	}
	decode(e.t, rec, &out)
	return e.caseID(cases.KindNotice)
}

func TestAddNotice(t *testing.T) {
	e := newEnv(t)
	for name, body := range map[string]string{
		"no name":         strings.Replace(noticeBody, `"Jane Doe"`, `"  "`, 1),
		"bad email":       strings.Replace(noticeBody, `jane@example.com`, `jane@example.com\r\nBcc: x@example.com`, 1),
		"future":          strings.Replace(noticeBody, `"RECEIVED"`, `"`+time.Now().Add(time.Hour).UTC().Format(time.RFC3339)+`"`, 1),
		"unknown account": strings.Replace(noticeBody, `"user_id":"2"`, `"user_id":"99"`, 1),
		"no explanation":  strings.Replace(noticeBody, `"It incites violence against a named person."`, `""`, 1),
	} {
		body = strings.Replace(body, "RECEIVED", time.Now().UTC().Format(time.RFC3339), 1)
		if rec := e.do(e.moderator, http.MethodPost, "/api/notices", body); rec.Code != http.StatusBadRequest {
			t.Fatalf("%s: %d %s", name, rec.Code, rec.Body)
		}
	}
	id := e.addNotice(noticeBody)
	list := e.listCases("?kind=notice")
	if len(list) != 1 || list[0].ID != id || list[0].Notice == nil {
		t.Fatalf("notice cases %+v", list)
	}
	n := list[0].Notice
	if n.NotifierName != "Jane Doe" || *n.NotifierEmail != "jane@example.com" || n.EnteredBy != "helper" || n.MailState != notices.MailNone {
		t.Fatalf("notice %+v", n)
	}
	if list[0].Subject == nil || list[0].Subject.Username != "spammer" || !list[0].Deadline.Equal(n.ReceivedAt.Add(cases.Deadline)) {
		t.Fatalf("subject and deadline %+v", list[0])
	}
	if audits := e.audit("notice.created"); len(audits) != 1 || audits[0]["has_email"] != true || audits[0]["user_id"] != "2" {
		t.Fatalf("audit %v", audits)
	}
	if rec := e.do(e.moderator, http.MethodPost, "/api/cases/"+itoa(id)+"/confirm", `{`+reason+`}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("a notice is not confirmed like an automatic level: %d", rec.Code)
	}
}

func TestDecideNoticeEmailsTheNotifier(t *testing.T) {
	e := newEnv(t)
	id := e.addNotice(noticeBody)
	path := "/api/cases/" + itoa(id) + "/notice-decision"
	if rec := e.do(e.moderator, http.MethodPost, path, `{"outcome":"maybe",`+reason+`}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown outcome %d", rec.Code)
	}
	rec := e.do(e.moderator, http.MethodPost, path, `{"outcome":"action_taken","reason_code":"illegal","reason_text":"We removed the message and suspended the account."}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"pending"`) {
		t.Fatalf("decide %d %s", rec.Code, rec.Body)
	}
	if rec := e.do(e.moderator, http.MethodPost, path, `{"outcome":"no_violation",`+reason+`}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("a decided notice %d", rec.Code)
	}
	if state := e.caseStates()[id]; state != cases.StateConfirmed {
		t.Fatalf("state %s", state)
	}
	sent, err := e.notices.SendDue(context.Background())
	if err != nil || sent != 1 {
		t.Fatalf("sent %d (%v)", sent, err)
	}
	m := e.mailer.sent[0]
	if m.To != "jane@example.com" || !strings.Contains(m.Body, "Hello Jane Doe") || !strings.Contains(m.Body, "We acted on the content you reported.") ||
		!strings.Contains(m.Body, "Rule: Illegal content") || !strings.Contains(m.Body, "We removed the message and suspended the account.") ||
		!strings.Contains(m.Body, "Group 'Town news'") {
		t.Fatalf("mail %+v", m)
	}
	if strings.Contains(m.Body, "helper") {
		t.Fatal("the mail names the moderator")
	}
	if again, _ := e.notices.SendDue(context.Background()); again != 0 {
		t.Fatalf("sent twice: %d", again)
	}
	n := e.listCases("?state=decided&kind=notice")[0].Notice
	if *n.Outcome != notices.OutcomeActionTaken || n.MailState != notices.MailSent || n.MailSentAt == nil {
		t.Fatalf("notice after the mail %+v", n)
	}
	if audits := e.audit("notice.decided"); len(audits) != 1 || audits[0]["outcome"] != "action_taken" || audits[0]["mail"] != "pending" {
		t.Fatalf("audit %v", audits)
	}
}

func TestNoticeMailRetriesThenFails(t *testing.T) {
	e := newEnv(t)
	id := e.addNotice(noticeBody)
	e.do(e.moderator, http.MethodPost, "/api/cases/"+itoa(id)+"/notice-decision", `{"outcome":"no_violation",`+reason+`}`)
	e.mailer.err = errors.New("mail server down")
	for range 6 {
		if _, err := e.notices.SendDue(context.Background()); err != nil {
			t.Fatal(err)
		}
		if _, err := e.mod.Exec(context.Background(), `UPDATE notices SET mail_next_at = now() - interval '1 second' WHERE mail_state = 'pending'`); err != nil {
			t.Fatal(err)
		}
	}
	n := e.listCases("?state=decided&kind=notice")[0].Notice
	if n.MailState != notices.MailFailed {
		t.Fatalf("mail state %s after 6 failures", n.MailState)
	}
}

func TestNoticeWithoutEmailOrWithoutSMTP(t *testing.T) {
	e := newEnv(t)
	quiet := e.addNotice(strings.Replace(noticeBody, `"notifier_email":"jane@example.com",`, ``, 1))
	rec := e.do(e.moderator, http.MethodPost, "/api/cases/"+itoa(quiet)+"/notice-decision", `{"outcome":"no_violation",`+reason+`}`)
	if !strings.Contains(rec.Body.String(), `"none"`) {
		t.Fatalf("no email: %d %s", rec.Code, rec.Body)
	}
	offline := notices.NewService(e.mod, e.app.Read, nil, nil)
	if offline.MailEnabled() {
		t.Fatal("mail enabled without a mailer")
	}
	other := e.addNotice(noticeBody)
	state, err := offline.Decide(context.Background(), e.staff("helper"), other, notices.Decision{Outcome: "no_violation", Reason: decisions.Reason{Code: "other", Text: "Nothing illegal."}})
	if err != nil || state != notices.MailSkipped {
		t.Fatalf("without SMTP: %s %v", state, err)
	}
	if sent, err := offline.SendDue(context.Background()); sent != 0 || err != nil {
		t.Fatalf("sending without SMTP: %d %v", sent, err)
	}
	if rec := e.do(e.moderator, http.MethodGet, "/api/notices/settings", ""); !strings.Contains(rec.Body.String(), `"mail_enabled":true`) {
		t.Fatalf("settings %s", rec.Body)
	}
}

func (e *env) staff(name string) staff.Staff {
	e.t.Helper()
	st := staff.Staff{Username: name, Role: staff.RoleModerator}
	if err := e.mod.QueryRow(context.Background(), `SELECT id FROM staff_accounts WHERE username = $1`, name).Scan(&st.ID); err != nil {
		e.t.Fatal(err)
	}
	return st
}
