package notices

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/mail"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/myronsi/messenger-moderation/internal/audit"
	"github.com/myronsi/messenger-moderation/internal/cases"
	"github.com/myronsi/messenger-moderation/internal/decisions"
	"github.com/myronsi/messenger-moderation/internal/staff"
)

const (
	OutcomeActionTaken = "action_taken"
	OutcomeNoViolation = "no_violation"

	MailNone    = "none"
	MailPending = "pending"
	MailSent    = "sent"
	MailFailed  = "failed"
	MailSkipped = "skipped"

	maxName        = 200
	maxEmail       = 254
	maxContent     = 2000
	maxLocation    = 1000
	maxExplanation = 4000
	clockSkew      = 5 * time.Minute
	oldestNotice   = 365 * 24 * time.Hour
)

var (
	ErrInvalid  = errors.New("invalid notice")
	ErrNotFound = errors.New("not found")
)

type Mailer interface {
	Send(ctx context.Context, to, subject, body string) error
}

type Service struct {
	mod  *pgxpool.Pool
	app  *pgxpool.Pool
	mail Mailer
	log  *slog.Logger
	now  func() time.Time
}

func NewService(mod, app *pgxpool.Pool, mailer Mailer, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{mod: mod, app: app, mail: mailer, log: log, now: time.Now}
}

func (s *Service) MailEnabled() bool { return s.mail != nil }

type Input struct {
	NotifierName  string    `json:"notifier_name"`
	NotifierEmail string    `json:"notifier_email"`
	Content       string    `json:"content"`
	Location      string    `json:"location"`
	Explanation   string    `json:"explanation"`
	ReceivedAt    time.Time `json:"received_at"`
	UserID        string    `json:"user_id"`
}

func cleanText(field, value string, limit int, required bool) (string, error) {
	t := strings.TrimSpace(value)
	if t == "" {
		if required {
			return "", fmt.Errorf("%w: %s is required", ErrInvalid, field)
		}
		return "", nil
	}
	if utf8.RuneCountInString(t) > limit {
		return "", fmt.Errorf("%w: %s must be at most %d characters", ErrInvalid, field, limit)
	}
	if strings.IndexFunc(t, func(c rune) bool { return unicode.IsControl(c) && c != '\n' }) >= 0 {
		return "", fmt.Errorf("%w: %s has control characters", ErrInvalid, field)
	}
	return t, nil
}

func CleanEmail(value string) (string, error) {
	t := strings.TrimSpace(value)
	if t == "" {
		return "", nil
	}
	parsed, err := mail.ParseAddress(t)
	if err != nil || parsed.Address != t || len(t) > maxEmail || strings.ContainsAny(t, "\r\n") {
		return "", fmt.Errorf("%w: notifier_email is not an email address", ErrInvalid)
	}
	return t, nil
}

type notice struct {
	name, email, content, location, explanation string
	receivedAt                                  time.Time
	userID                                      *int64
}

func (s *Service) clean(ctx context.Context, in Input) (notice, error) {
	var n notice
	var err error
	if n.name, err = cleanText("notifier_name", in.NotifierName, maxName, true); err != nil {
		return n, err
	}
	if n.email, err = CleanEmail(in.NotifierEmail); err != nil {
		return n, err
	}
	if n.content, err = cleanText("content", in.Content, maxContent, true); err != nil {
		return n, err
	}
	if n.location, err = cleanText("location", in.Location, maxLocation, true); err != nil {
		return n, err
	}
	if n.explanation, err = cleanText("explanation", in.Explanation, maxExplanation, true); err != nil {
		return n, err
	}
	now := s.now()
	if in.ReceivedAt.IsZero() || in.ReceivedAt.After(now.Add(clockSkew)) || in.ReceivedAt.Before(now.Add(-oldestNotice)) {
		return n, fmt.Errorf("%w: received_at must be a time in the last year, not in the future", ErrInvalid)
	}
	n.receivedAt = in.ReceivedAt.UTC()
	if raw := strings.TrimSpace(in.UserID); raw != "" {
		id, err := strconv.ParseInt(strings.TrimPrefix(raw, "#"), 10, 64)
		if err != nil || id <= 0 {
			return n, fmt.Errorf("%w: user_id must be an account ID", ErrInvalid)
		}
		if s.app != nil {
			var exists bool
			if err := s.app.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM users WHERE id = $1)`, id).Scan(&exists); err != nil {
				return n, err
			}
			if !exists {
				return n, fmt.Errorf("%w: no account with this ID", ErrInvalid)
			}
		}
		n.userID = &id
	}
	return n, nil
}

func sourceKey() (string, error) {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "notice:" + hex.EncodeToString(b), nil
}

func (s *Service) Create(ctx context.Context, st staff.Staff, in Input) (int64, error) {
	n, err := s.clean(ctx, in)
	if err != nil {
		return 0, err
	}
	key, err := sourceKey()
	if err != nil {
		return 0, err
	}
	tx, err := s.mod.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var id int64
	if err := tx.QueryRow(ctx, `INSERT INTO cases (kind, source_key, user_id, opened_at, deadline)
VALUES ('notice', $1, $2, $3, $4) RETURNING id`, key, n.userID, n.receivedAt, n.receivedAt.Add(cases.Deadline)).Scan(&id); err != nil {
		return 0, err
	}
	var email *string
	if n.email != "" {
		email = &n.email
	}
	if _, err := tx.Exec(ctx, `INSERT INTO notices (case_id, notifier_name, notifier_email, content, location, explanation, received_at, entered_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`, id, n.name, email, n.content, n.location, n.explanation, n.receivedAt, st.ID); err != nil {
		return 0, err
	}
	details := map[string]any{"received_at": n.receivedAt, "has_email": email != nil}
	if n.userID != nil {
		details["user_id"] = strconv.FormatInt(*n.userID, 10)
	}
	if err := audit.Write(ctx, tx, audit.Entry{StaffID: &st.ID, Action: "notice.created", TargetType: "case", TargetID: strconv.FormatInt(id, 10), Details: details}); err != nil {
		return 0, err
	}
	return id, tx.Commit(ctx)
}

type Decision struct {
	Outcome string `json:"outcome"`
	decisions.Reason
}

func (s *Service) Decide(ctx context.Context, st staff.Staff, caseID int64, d Decision) (string, error) {
	if d.Outcome != OutcomeActionTaken && d.Outcome != OutcomeNoViolation {
		return "", fmt.Errorf("%w: outcome must be action_taken or no_violation", ErrInvalid)
	}
	reason, err := d.Clean()
	if err != nil {
		return "", fmt.Errorf("%w: %s", ErrInvalid, strings.TrimPrefix(err.Error(), decisions.ErrInvalid.Error()+": "))
	}
	tx, err := s.mod.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var kind, state string
	var email *string
	err = tx.QueryRow(ctx, `SELECT c.kind, c.state, n.notifier_email FROM cases c LEFT JOIN notices n ON n.case_id = c.id
WHERE c.id = $1 FOR UPDATE OF c`, caseID).Scan(&kind, &state, &email)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	if kind != cases.KindNotice {
		return "", fmt.Errorf("%w: this case is not a notice", ErrInvalid)
	}
	if state != cases.StateOpen {
		return "", fmt.Errorf("%w: the case is already closed", ErrInvalid)
	}
	mailState := MailNone
	switch {
	case email == nil:
	case s.mail == nil:
		mailState = MailSkipped
	default:
		mailState = MailPending
	}
	now := s.now()
	caseState := cases.StateConfirmed
	if d.Outcome == OutcomeNoViolation {
		caseState = cases.StateReversed
	}
	if _, err := tx.Exec(ctx, `UPDATE cases SET state = $2, decided_at = $3, decided_by = $4, reason_code = $5, reason_text = $6 WHERE id = $1`,
		caseID, caseState, now, st.ID, reason.Code, reason.Text); err != nil {
		return "", err
	}
	if _, err := tx.Exec(ctx, `UPDATE notices SET outcome = $2, mail_state = $3, mail_next_at = CASE WHEN $3 = 'pending' THEN $4::timestamptz END WHERE case_id = $1`,
		caseID, d.Outcome, mailState, now); err != nil {
		return "", err
	}
	if err := audit.Write(ctx, tx, audit.Entry{StaffID: &st.ID, Action: "notice.decided", TargetType: "case", TargetID: strconv.FormatInt(caseID, 10),
		Details: map[string]any{"outcome": d.Outcome, "reason_code": reason.Code, "reason_text": reason.Text, "mail": mailState}}); err != nil {
		return "", err
	}
	return mailState, tx.Commit(ctx)
}
