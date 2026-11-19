package decisions

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/myronsi/messenger-moderation/internal/audit"
	"github.com/myronsi/messenger-moderation/internal/cases"
	"github.com/myronsi/messenger-moderation/internal/messenger"
	"github.com/myronsi/messenger-moderation/internal/staff"
)

const (
	MaxReasonText = 1000
	CaseListLimit = 200
)

var (
	ErrInvalid     = errors.New("invalid request")
	ErrForbidden   = errors.New("not allowed for this role")
	ErrNotFound    = errors.New("not found")
	ErrUnavailable = errors.New("messenger is not connected")
)

var (
	ReasonCodes = []string{"spam", "scam", "harassment", "impersonation", "inappropriate_profile", "illegal", "other"}
	Statuses    = []string{"good", "warned", "limited", "restricted", "suspended"}
	PackKinds   = []string{"sticker_pack", "emoji_pack"}
)

type Messenger interface {
	SetStatus(ctx context.Context, userID int64, req messenger.StatusRequest) (messenger.Change, error)
	DismissReports(ctx context.Context, userID int64, reporterIDs []int64) (messenger.Dismissal, error)
	SetPackHidden(ctx context.Context, kind string, packID int64, hidden bool) (bool, error)
	DeleteAccount(ctx context.Context, userID int64) error
	PackImage(ctx context.Context, attachmentID string) (*http.Response, error)
}

type Reason struct {
	Code string `json:"reason_code"`
	Text string `json:"reason_text"`
}

func (r Reason) clean() (Reason, error) {
	if !slices.Contains(ReasonCodes, r.Code) {
		return r, fmt.Errorf("%w: choose a rule from the guidelines", ErrInvalid)
	}
	t := strings.TrimSpace(r.Text)
	if t == "" || utf8.RuneCountInString(t) > MaxReasonText {
		return r, fmt.Errorf("%w: the text for the user must be 1 to %d characters", ErrInvalid, MaxReasonText)
	}
	if strings.IndexFunc(t, func(c rune) bool { return unicode.IsControl(c) && c != '\n' }) >= 0 {
		return r, fmt.Errorf("%w: the text for the user has control characters", ErrInvalid)
	}
	r.Text = t
	return r, nil
}

type Service struct {
	mod       *pgxpool.Pool
	app       *pgxpool.Pool
	messenger Messenger
	cases     *cases.Store
	log       *slog.Logger
	now       func() time.Time
}

func NewService(mod, app *pgxpool.Pool, m Messenger, c *cases.Store, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{mod: mod, app: app, messenger: m, cases: c, log: log, now: time.Now}
}

func staffRef(st staff.Staff) string { return "staff:" + strconv.FormatInt(st.ID, 10) }

func userTarget(id int64) string { return strconv.FormatInt(id, 10) }

func (s *Service) messengerError(err error) error {
	var field *messenger.FieldError
	switch {
	case errors.As(err, &field):
		return fmt.Errorf("%w: %s", ErrInvalid, field.Error())
	case errors.Is(err, messenger.ErrNotFound):
		return ErrNotFound
	case errors.Is(err, messenger.ErrForbidden):
		return fmt.Errorf("%w: Messenger does not allow this", ErrForbidden)
	case errors.Is(err, messenger.ErrNotConfigured):
		return ErrUnavailable
	}
	return err
}

func (s *Service) record(ctx context.Context, st staff.Staff, e audit.Entry, close func(pgx.Tx, cases.Decision) ([]int64, error), r Reason) error {
	tx, err := s.mod.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if close != nil {
		closed, err := close(tx, cases.Decision{StaffID: st.ID, ReasonCode: r.Code, ReasonText: r.Text, At: s.now()})
		if err != nil {
			return err
		}
		if len(closed) > 0 {
			ids := make([]string, len(closed))
			for i, id := range closed {
				ids[i] = strconv.FormatInt(id, 10)
			}
			e.Details["closed_cases"] = ids
		}
	}
	e.StaffID = &st.ID
	if err := audit.Write(ctx, tx, e); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) recordOrLog(ctx context.Context, st staff.Staff, e audit.Entry, close func(pgx.Tx, cases.Decision) ([]int64, error), r Reason) error {
	if err := s.record(ctx, st, e, close, r); err != nil {
		s.log.ErrorContext(ctx, "decision applied in Messenger but not recorded", "staff_id", st.ID, "action", e.Action,
			"target", e.TargetType+":"+e.TargetID, "details", e.Details, "error", err)
		return fmt.Errorf("the decision was applied but could not be recorded: %w", err)
	}
	return nil
}

func (s *Service) SetStatus(ctx context.Context, st staff.Staff, userID int64, status string, r Reason) (messenger.Change, error) {
	if !slices.Contains(Statuses, status) {
		return messenger.Change{}, fmt.Errorf("%w: unknown status", ErrInvalid)
	}
	r, err := r.clean()
	if err != nil {
		return messenger.Change{}, err
	}
	change, err := s.messenger.SetStatus(ctx, userID, messenger.StatusRequest{Status: status, ReasonCode: r.Code, ReasonText: r.Text, StaffRef: staffRef(st)})
	if err != nil {
		return messenger.Change{}, s.messengerError(err)
	}
	err = s.recordOrLog(ctx, st, audit.Entry{
		Action: "user.status_set", TargetType: "user", TargetID: userTarget(userID),
		Details: map[string]any{"before": change.Before, "after": change.After, "reason_code": r.Code, "reason_text": r.Text},
	}, func(tx pgx.Tx, d cases.Decision) ([]int64, error) { return cases.CloseUser(ctx, tx, userID, status, d) }, r)
	return change, err
}

func (s *Service) DismissReports(ctx context.Context, st staff.Staff, userID int64, reporterIDs []int64, r Reason) (messenger.Dismissal, error) {
	r, err := r.clean()
	if err != nil {
		return messenger.Dismissal{}, err
	}
	if reporterIDs != nil && len(reporterIDs) == 0 {
		return messenger.Dismissal{}, fmt.Errorf("%w: choose at least one report", ErrInvalid)
	}
	res, err := s.messenger.DismissReports(ctx, userID, reporterIDs)
	if err != nil {
		return messenger.Dismissal{}, s.messengerError(err)
	}
	details := map[string]any{"dismissed": res.Dismissed, "before": res.Before, "after": res.After, "reason_code": r.Code, "reason_text": r.Text}
	if reporterIDs != nil {
		ids := make([]string, len(reporterIDs))
		for i, id := range reporterIDs {
			ids[i] = strconv.FormatInt(id, 10)
		}
		details["reporter_ids"] = ids
	}
	err = s.recordOrLog(ctx, st, audit.Entry{Action: "user.reports_dismissed", TargetType: "user", TargetID: userTarget(userID), Details: details}, nil, r)
	return res, err
}

func (s *Service) SetPackHidden(ctx context.Context, st staff.Staff, kind string, packID int64, hidden bool, r Reason) (bool, error) {
	if !slices.Contains(PackKinds, kind) {
		return false, fmt.Errorf("%w: unknown pack kind", ErrInvalid)
	}
	r, err := r.clean()
	if err != nil {
		return false, err
	}
	changed, err := s.messenger.SetPackHidden(ctx, kind, packID, hidden)
	if err != nil {
		return false, s.messengerError(err)
	}
	action, state := "pack.unhidden", cases.StateReversed
	if hidden {
		action, state = "pack.hidden", cases.StateConfirmed
	}
	err = s.recordOrLog(ctx, st, audit.Entry{
		Action: action, TargetType: kind, TargetID: strconv.FormatInt(packID, 10),
		Details: map[string]any{"changed": changed, "reason_code": r.Code, "reason_text": r.Text},
	}, func(tx pgx.Tx, d cases.Decision) ([]int64, error) {
		return cases.ClosePack(ctx, tx, kind, packID, state, d)
	}, r)
	return changed, err
}

func (s *Service) DeleteAccount(ctx context.Context, st staff.Staff, userID int64, r Reason) error {
	if !st.IsAdmin() {
		return ErrForbidden
	}
	r, err := r.clean()
	if err != nil {
		return err
	}
	if err := s.messenger.DeleteAccount(ctx, userID); err != nil {
		return s.messengerError(err)
	}
	return s.recordOrLog(ctx, st, audit.Entry{
		Action: "user.deleted", TargetType: "user", TargetID: userTarget(userID),
		Details: map[string]any{"reason_code": r.Code, "reason_text": r.Text},
	}, func(tx pgx.Tx, d cases.Decision) ([]int64, error) {
		return cases.CloseUser(ctx, tx, userID, "suspended", d)
	}, r)
}

func (s *Service) ConfirmCase(ctx context.Context, st staff.Staff, caseID int64, r Reason) error {
	c, err := s.cases.Get(ctx, caseID)
	if errors.Is(err, cases.ErrNotFound) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if c.State != cases.StateOpen {
		return fmt.Errorf("%w: the case is already closed", ErrInvalid)
	}
	switch c.Kind {
	case cases.KindAccountStatus:
		_, err = s.SetStatus(ctx, st, *c.UserID, *c.ToStatus, r)
	default:
		_, err = s.SetPackHidden(ctx, st, *c.PackKind, *c.PackID, true, r)
	}
	return err
}

func (s *Service) ReverseCase(ctx context.Context, st staff.Staff, caseID int64, status string, r Reason) error {
	c, err := s.cases.Get(ctx, caseID)
	if errors.Is(err, cases.ErrNotFound) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if c.State != cases.StateOpen {
		return fmt.Errorf("%w: the case is already closed", ErrInvalid)
	}
	switch c.Kind {
	case cases.KindAccountStatus:
		if !slices.Contains(Statuses, status) || cases.Level(status) >= cases.Level(*c.ToStatus) {
			return fmt.Errorf("%w: choose a status below %s", ErrInvalid, *c.ToStatus)
		}
		_, err = s.SetStatus(ctx, st, *c.UserID, status, r)
	default:
		_, err = s.SetPackHidden(ctx, st, *c.PackKind, *c.PackID, false, r)
	}
	return err
}
