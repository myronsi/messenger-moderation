package notices

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	maxMailAttempts = 6
	mailBatch       = 20
)

var ruleNames = map[string]string{
	"spam":                  "Spam",
	"scam":                  "Scam or fraud",
	"harassment":            "Harassment or bullying",
	"impersonation":         "Pretending to be someone else",
	"inappropriate_profile": "Inappropriate name, picture or bio",
	"illegal":               "Illegal content",
	"other":                 "Other breach of the Community Guidelines",
}

type Reply struct {
	NotifierName string
	ReceivedAt   time.Time
	Location     string
	Outcome      string
	ReasonCode   string
	ReasonText   string
}

func Compose(r Reply) (subject, body string) {
	subject = "Decision on your notice to Messenger"
	decision := "We found no violation of the law or of our Community Guidelines, so we did not act on the content."
	if r.Outcome == OutcomeActionTaken {
		decision = "We acted on the content you reported."
	}
	rule := ruleNames[r.ReasonCode]
	if rule == "" {
		rule = r.ReasonCode
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Hello %s,\n\n", r.NotifierName)
	fmt.Fprintf(&b, "a person on the Messenger moderation team reviewed your notice of %s (UTC) about:\n\n", r.ReceivedAt.UTC().Format("2 January 2006, 15:04"))
	fmt.Fprintf(&b, "%s\n\n", r.Location)
	fmt.Fprintf(&b, "Decision: %s\n", decision)
	fmt.Fprintf(&b, "Rule: %s\n\n", rule)
	fmt.Fprintf(&b, "%s\n\n", r.ReasonText)
	b.WriteString("If you disagree with this decision, you can write to us again with your reasons. You can also turn to a certified out-of-court dispute settlement body or to a court.\n\n")
	b.WriteString("Messenger moderation\n")
	return subject, b.String()
}

type SMTP struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
	send     func(addr string, auth smtp.Auth, from string, to []string, msg []byte) error
}

func NewSMTP(host string, port int, username, password, from string) *SMTP {
	return &SMTP{Host: host, Port: port, Username: username, Password: password, From: from, send: smtp.SendMail}
}

func (m *SMTP) Send(_ context.Context, to, subject, body string) error {
	if strings.ContainsAny(to+subject, "\r\n") {
		return errors.New("invalid mail header")
	}
	if parsed, err := mail.ParseAddress(to); err != nil || parsed.Address != to {
		return errors.New("invalid recipient")
	}
	headers := []string{
		"From: " + m.From,
		"To: " + to,
		"Subject: " + subject,
		"Date: " + time.Now().UTC().Format(time.RFC1123Z),
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=utf-8",
	}
	msg := strings.Join(headers, "\r\n") + "\r\n\r\n" + strings.ReplaceAll(body, "\n", "\r\n")
	var auth smtp.Auth
	if m.Username != "" {
		auth = smtp.PlainAuth("", m.Username, m.Password, m.Host)
	}
	return m.send(net.JoinHostPort(m.Host, strconv.Itoa(m.Port)), auth, m.From, []string{to}, []byte(msg))
}

type pendingMail struct {
	CaseID     int64
	Attempts   int
	Name       string
	Email      string
	Location   string
	ReceivedAt time.Time
	Outcome    string
	ReasonCode string
	ReasonText string
}

func (s *Service) SendDue(ctx context.Context) (sent int, err error) {
	if s.mail == nil {
		return 0, nil
	}
	for range mailBatch {
		var p pendingMail
		err := s.mod.QueryRow(ctx, `UPDATE notices n SET mail_attempts = n.mail_attempts + 1, mail_next_at = now() + interval '10 minutes'
FROM cases c
WHERE n.case_id = (SELECT case_id FROM notices WHERE mail_state = 'pending' AND mail_next_at <= now()
                   ORDER BY mail_next_at FOR UPDATE SKIP LOCKED LIMIT 1)
  AND c.id = n.case_id
RETURNING n.case_id, n.mail_attempts, n.notifier_name, n.notifier_email, n.location, n.received_at, n.outcome, c.reason_code, c.reason_text`).
			Scan(&p.CaseID, &p.Attempts, &p.Name, &p.Email, &p.Location, &p.ReceivedAt, &p.Outcome, &p.ReasonCode, &p.ReasonText)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return sent, nil
			}
			return sent, err
		}
		subject, body := Compose(Reply{NotifierName: p.Name, ReceivedAt: p.ReceivedAt, Location: p.Location, Outcome: p.Outcome, ReasonCode: p.ReasonCode, ReasonText: p.ReasonText})
		if sendErr := s.mail.Send(ctx, p.Email, subject, body); sendErr != nil {
			s.log.WarnContext(ctx, "notice reply not sent", "case_id", p.CaseID, "attempt", p.Attempts, "error", sendErr)
			if err := s.retryLater(ctx, p); err != nil {
				return sent, err
			}
			continue
		}
		if _, err := s.mod.Exec(ctx, `UPDATE notices SET mail_state = 'sent', mail_sent_at = now(), mail_next_at = NULL WHERE case_id = $1`, p.CaseID); err != nil {
			return sent, err
		}
		sent++
	}
	return sent, nil
}

func (s *Service) retryLater(ctx context.Context, p pendingMail) error {
	if p.Attempts >= maxMailAttempts {
		_, err := s.mod.Exec(ctx, `UPDATE notices SET mail_state = 'failed', mail_next_at = NULL WHERE case_id = $1`, p.CaseID)
		return err
	}
	delay := time.Duration(1<<p.Attempts) * time.Minute
	_, err := s.mod.Exec(ctx, `UPDATE notices SET mail_next_at = now() + make_interval(secs => $2) WHERE case_id = $1`, p.CaseID, delay.Seconds())
	return err
}

func (s *Service) RunMail(ctx context.Context, every time.Duration) {
	if s.mail == nil {
		return
	}
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		if n, err := s.SendDue(ctx); err != nil && ctx.Err() == nil {
			s.log.ErrorContext(ctx, "notice replies failed", "error", err)
		} else if n > 0 {
			s.log.InfoContext(ctx, "notice replies sent", "count", n)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
