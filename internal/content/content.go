package content

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/myronsi/messenger-moderation/internal/audit"
	"github.com/myronsi/messenger-moderation/internal/messenger"
	"github.com/myronsi/messenger-moderation/internal/staff"
)

const (
	ListLimit     = 200
	MaxReasonText = 1000
	MaxNote       = 1000
	ActionRemoved = "removed"
	ActionNone    = "no_violation"
)

var (
	ErrInvalid     = errors.New("invalid request")
	ErrNotFound    = errors.New("not found")
	ErrUnavailable = errors.New("messenger is not connected")
	ReasonCodes    = []string{"spam", "scam", "harassment", "impersonation", "inappropriate_profile", "illegal", "other"}
)

type Remover interface {
	RemoveMessage(ctx context.Context, chatID, messageID int64, reasonCode, reasonText string) (messenger.Removal, error)
}

type Person struct {
	ID          int64  `json:"id,string"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
}

type Decision struct {
	Action     string    `json:"action"`
	StaffID    int64     `json:"staff_id,string"`
	StaffName  string    `json:"staff_username"`
	ReasonCode *string   `json:"reason_code"`
	ReasonText *string   `json:"reason_text"`
	DecidedAt  time.Time `json:"decided_at"`
}

type Summary struct {
	ID          int64      `json:"id,string"`
	Kind        string     `json:"kind"`
	ChatID      int64      `json:"chat_id,string"`
	MessageID   int64      `json:"message_id,string"`
	Reason      string     `json:"reason"`
	Note        *string    `json:"note"`
	Reporter    *Person    `json:"reporter"`
	Target      *Person    `json:"target"`
	ChatTitle   *string    `json:"chat_title"`
	Excerpt     string     `json:"excerpt"`
	CreatedAt   time.Time  `json:"created_at"`
	WithdrawnAt *time.Time `json:"withdrawn_at"`
	Reports     int        `json:"reports_on_item"`
	Decision    *Decision  `json:"decision"`
}

type Detail struct {
	Summary
	Copy   json.RawMessage `json:"copy"`
	Others []Summary       `json:"other_reports"`
}

type copyShape struct {
	Chat struct {
		Title *string `json:"title"`
	} `json:"chat"`
	Messages []copiedMessage `json:"messages"`
	Group    *struct {
		Title string `json:"title"`
	} `json:"group"`
}

type copiedMessage struct {
	Text     *string  `json:"text"`
	Files    []string `json:"files"`
	Reported bool     `json:"reported"`
	Poll     *struct {
		Question string `json:"question"`
	} `json:"poll"`
	Location *struct {
		Title string `json:"title"`
	} `json:"location"`
	Contact *struct {
		Name string `json:"name"`
	} `json:"contact"`
}

func (m copiedMessage) summary() string {
	switch {
	case m.Poll != nil && strings.TrimSpace(m.Poll.Question) != "":
		return "📊 " + m.Poll.Question
	case m.Location != nil && strings.TrimSpace(m.Location.Title) != "":
		return "📍 " + m.Location.Title
	case m.Contact != nil && strings.TrimSpace(m.Contact.Name) != "":
		return "👤 " + m.Contact.Name
	}
	if m.Text != nil && *m.Text != "" {
		return *m.Text
	}
	return strings.Join(m.Files, ", ")
}

func excerpt(raw []byte) (string, *string) {
	var c copyShape
	if json.Unmarshal(raw, &c) != nil {
		return "", nil
	}
	if c.Group != nil {
		return c.Group.Title, c.Chat.Title
	}
	for _, m := range c.Messages {
		if !m.Reported {
			continue
		}
		text := m.summary()
		runes := []rune(text)
		if len(runes) > 140 {
			text = string(runes[:140]) + "…"
		}
		return text, c.Chat.Title
	}
	return "", c.Chat.Title
}

type Service struct {
	mod     *pgxpool.Pool
	app     *pgxpool.Pool
	remover Remover
	log     *slog.Logger
	now     func() time.Time
}

func NewService(mod, app *pgxpool.Pool, r Remover, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{mod: mod, app: app, remover: r, log: log, now: time.Now}
}

type itemKey struct{ chat, message int64 }

func (s *Service) decisions(ctx context.Context) (map[itemKey]Decision, error) {
	rows, err := s.mod.Query(ctx, `
		SELECT d.chat_id, d.message_id, d.action, d.staff_id, s.username, d.reason_code, d.reason_text, d.decided_at
		FROM content_decisions d JOIN staff_accounts s ON s.id = d.staff_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[itemKey]Decision{}
	for rows.Next() {
		var k itemKey
		var d Decision
		if err := rows.Scan(&k.chat, &k.message, &d.Action, &d.StaffID, &d.StaffName, &d.ReasonCode, &d.ReasonText, &d.DecidedAt); err != nil {
			return nil, err
		}
		out[k] = d
	}
	return out, rows.Err()
}

const summaryColumns = `
	cr.id, cr.kind, cr.chat_id, cr.message_id, cr.reason, cr.note, cr.content, cr.created_at, cr.withdrawn_at,
	rep.id, rep.username, rep.display_name, tgt.id, tgt.username, tgt.display_name,
	(SELECT count(*) FROM content_reports x WHERE x.chat_id = cr.chat_id AND x.message_id = cr.message_id AND x.withdrawn_at IS NULL)`

const summaryFrom = `
	FROM content_reports cr
	LEFT JOIN users rep ON rep.id = cr.reporter_id
	LEFT JOIN users tgt ON tgt.id = cr.target_id`

func scanSummary(row pgx.Row) (Summary, []byte, error) {
	var (
		s                Summary
		raw              []byte
		repID, tgtID     *int64
		repUser, repName *string
		tgtUser, tgtName *string
		reports          int64
	)
	if err := row.Scan(&s.ID, &s.Kind, &s.ChatID, &s.MessageID, &s.Reason, &s.Note, &raw, &s.CreatedAt, &s.WithdrawnAt,
		&repID, &repUser, &repName, &tgtID, &tgtUser, &tgtName, &reports); err != nil {
		return Summary{}, nil, err
	}
	person := func(id *int64, user, name *string) *Person {
		if id == nil {
			return nil
		}
		p := &Person{ID: *id}
		if user != nil {
			p.Username = *user
		}
		if name != nil {
			p.DisplayName = *name
		}
		return p
	}
	s.Reporter, s.Target, s.Reports = person(repID, repUser, repName), person(tgtID, tgtUser, tgtName), int(reports)
	s.Excerpt, s.ChatTitle = excerpt(raw)
	return s, raw, nil
}

func (s *Service) List(ctx context.Context, st staff.Staff, decided bool) ([]Summary, error) {
	if s.app == nil {
		return nil, ErrUnavailable
	}
	done, err := s.decisions(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.app.Query(ctx, `SELECT `+summaryColumns+summaryFrom+` WHERE cr.withdrawn_at IS NULL ORDER BY cr.created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Summary{}
	seen := map[itemKey]bool{}
	for rows.Next() {
		item, _, err := scanSummary(rows)
		if err != nil {
			return nil, err
		}
		k := itemKey{item.ChatID, item.MessageID}
		d, isDecided := done[k]
		if isDecided != decided || seen[k] {
			continue
		}
		seen[k] = true
		if isDecided {
			d := d
			item.Decision = &d
		}
		out = append(out, item)
		if len(out) == ListLimit {
			break
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	ids := make([]string, len(out))
	for i, item := range out {
		ids[i] = strconv.FormatInt(item.ID, 10)
	}
	state := "open"
	if decided {
		state = "decided"
	}
	if err := audit.Write(ctx, s.mod, audit.Entry{StaffID: &st.ID, Action: "content_reports.viewed", Details: map[string]any{"state": state, "report_ids": ids}}); err != nil {
		return nil, fmt.Errorf("data access log: %w", err)
	}
	return out, nil
}

func (s *Service) load(ctx context.Context, id int64) (Detail, error) {
	if s.app == nil {
		return Detail{}, ErrUnavailable
	}
	row := s.app.QueryRow(ctx, `SELECT `+summaryColumns+summaryFrom+` WHERE cr.id = $1`, id)
	item, raw, err := scanSummary(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Detail{}, ErrNotFound
	}
	if err != nil {
		return Detail{}, err
	}
	d := Detail{Summary: item, Copy: raw, Others: []Summary{}}
	rows, err := s.app.Query(ctx, `SELECT `+summaryColumns+summaryFrom+`
		WHERE cr.chat_id = $1 AND cr.message_id = $2 AND cr.id <> $3 ORDER BY cr.created_at`, item.ChatID, item.MessageID, item.ID)
	if err != nil {
		return Detail{}, err
	}
	defer rows.Close()
	for rows.Next() {
		other, _, err := scanSummary(rows)
		if err != nil {
			return Detail{}, err
		}
		d.Others = append(d.Others, other)
	}
	if err := rows.Err(); err != nil {
		return Detail{}, err
	}
	var dec Decision
	err = s.mod.QueryRow(ctx, `
		SELECT d.action, d.staff_id, s.username, d.reason_code, d.reason_text, d.decided_at
		FROM content_decisions d JOIN staff_accounts s ON s.id = d.staff_id
		WHERE d.chat_id = $1 AND d.message_id = $2`, item.ChatID, item.MessageID).
		Scan(&dec.Action, &dec.StaffID, &dec.StaffName, &dec.ReasonCode, &dec.ReasonText, &dec.DecidedAt)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
	case err != nil:
		return Detail{}, err
	default:
		d.Decision = &dec
	}
	return d, nil
}

func target(id int64) string { return strconv.FormatInt(id, 10) }

func (s *Service) Get(ctx context.Context, st staff.Staff, id int64) (Detail, error) {
	d, err := s.load(ctx, id)
	if err != nil {
		return Detail{}, err
	}
	if err := audit.Write(ctx, s.mod, audit.Entry{StaffID: &st.ID, Action: "content_report.viewed", TargetType: "content_report", TargetID: target(id)}); err != nil {
		return Detail{}, fmt.Errorf("data access log: %w", err)
	}
	return d, nil
}

type Reason struct {
	Code string `json:"reason_code"`
	Text string `json:"reason_text"`
}

func cleanText(t string, limit int, required bool, what string) (string, error) {
	t = strings.TrimSpace(t)
	if required && t == "" {
		return "", fmt.Errorf("%w: %s is required", ErrInvalid, what)
	}
	if utf8.RuneCountInString(t) > limit {
		return "", fmt.Errorf("%w: %s is at most %d characters", ErrInvalid, what, limit)
	}
	if strings.IndexFunc(t, func(c rune) bool { return unicode.IsControl(c) && c != '\n' }) >= 0 {
		return "", fmt.Errorf("%w: %s has control characters", ErrInvalid, what)
	}
	return t, nil
}

func (s *Service) messengerError(err error) error {
	var field *messenger.FieldError
	switch {
	case errors.As(err, &field):
		return fmt.Errorf("%w: %s", ErrInvalid, field.Error())
	case errors.Is(err, messenger.ErrNotFound):
		return fmt.Errorf("%w: the message no longer exists in Messenger", ErrNotFound)
	case errors.Is(err, messenger.ErrNotConfigured):
		return ErrUnavailable
	}
	return err
}

func (s *Service) decide(ctx context.Context, st staff.Staff, d Detail, action string, code, text *string, e audit.Entry) error {
	tx, err := s.mod.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `
		INSERT INTO content_decisions (chat_id, message_id, action, staff_id, reason_code, reason_text, decided_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (chat_id, message_id) DO UPDATE SET action = EXCLUDED.action, staff_id = EXCLUDED.staff_id,
			reason_code = EXCLUDED.reason_code, reason_text = EXCLUDED.reason_text, decided_at = EXCLUDED.decided_at`,
		d.ChatID, d.MessageID, action, st.ID, code, text, s.now()); err != nil {
		return err
	}
	e.StaffID = &st.ID
	if err := audit.Write(ctx, tx, e); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) Remove(ctx context.Context, st staff.Staff, id int64, r Reason) (messenger.Removal, error) {
	if !slices.Contains(ReasonCodes, r.Code) {
		return messenger.Removal{}, fmt.Errorf("%w: choose a rule from the guidelines", ErrInvalid)
	}
	text, err := cleanText(r.Text, MaxReasonText, true, "the text for the sender")
	if err != nil {
		return messenger.Removal{}, err
	}
	d, err := s.load(ctx, id)
	if err != nil {
		return messenger.Removal{}, err
	}
	if d.Kind != "message" {
		return messenger.Removal{}, fmt.Errorf("%w: only reported messages can be removed", ErrInvalid)
	}
	if s.remover == nil {
		return messenger.Removal{}, ErrUnavailable
	}
	res, err := s.remover.RemoveMessage(ctx, d.ChatID, d.MessageID, r.Code, text)
	if err != nil {
		return messenger.Removal{}, s.messengerError(err)
	}
	details := map[string]any{
		"report_id": target(id), "chat_id": target(d.ChatID), "message_id": target(d.MessageID),
		"reason_code": r.Code, "reason_text": text, "already_removed": res.AlreadyRemoved,
	}
	if res.SenderID != nil {
		details["sender_id"] = *res.SenderID
	}
	if err := s.decide(ctx, st, d, ActionRemoved, &r.Code, &text, audit.Entry{
		Action: "message.removed", TargetType: "message", TargetID: target(d.ChatID) + ":" + target(d.MessageID), Details: details,
	}); err != nil {
		s.log.ErrorContext(ctx, "message removed in Messenger but not recorded", "staff_id", st.ID, "report_id", id, "error", err)
		return res, fmt.Errorf("the message was removed but the decision could not be recorded: %w", err)
	}
	return res, nil
}

func (s *Service) Dismiss(ctx context.Context, st staff.Staff, id int64, note string) error {
	note, err := cleanText(note, MaxNote, false, "the note")
	if err != nil {
		return err
	}
	d, err := s.load(ctx, id)
	if err != nil {
		return err
	}
	var text *string
	if note != "" {
		text = &note
	}
	return s.decide(ctx, st, d, ActionNone, nil, text, audit.Entry{
		Action: "content_report.dismissed", TargetType: "content_report", TargetID: target(id),
		Details: map[string]any{"chat_id": target(d.ChatID), "message_id": target(d.MessageID), "kind": d.Kind, "note": note},
	})
}
