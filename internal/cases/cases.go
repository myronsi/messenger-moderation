package cases

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	Deadline = 72 * time.Hour

	KindAccountStatus = "account_status"
	KindPackHidden    = "pack_hidden"
	KindReviewRequest = "review_request"
	KindNotice        = "notice"

	StateOpen       = "open"
	StateConfirmed  = "confirmed"
	StateReversed   = "reversed"
	StateSuperseded = "superseded"

	MaxAge        = 30 * 24 * time.Hour
	watchEvents   = "status_events"
	watchRequests = "review_requests"
	eventBatch    = 500
	eventLag      = 200
)

var ErrNotFound = errors.New("case not found")

var Kinds = []string{KindAccountStatus, KindPackHidden, KindReviewRequest, KindNotice}

var levels = map[string]int{"good": 0, "warned": 1, "limited": 2, "restricted": 3, "suspended": 4}

func Level(status string) int { return levels[status] }

type Case struct {
	ID          int64      `json:"id,string"`
	Kind        string     `json:"kind"`
	UserID      *int64     `json:"user_id,string,omitempty"`
	PackKind    *string    `json:"pack_kind,omitempty"`
	PackID      *int64     `json:"pack_id,string,omitempty"`
	FromStatus  *string    `json:"from_status,omitempty"`
	ToStatus    *string    `json:"to_status,omitempty"`
	OpenedAt    time.Time  `json:"opened_at"`
	Deadline    time.Time  `json:"deadline"`
	State       string     `json:"state"`
	DecidedAt   *time.Time `json:"decided_at"`
	DecidedBy   *string    `json:"decided_by"`
	ReasonCode  *string    `json:"reason_code"`
	ReasonText  *string    `json:"reason_text"`
	RequestText *string    `json:"request_text,omitempty"`
}

type Store struct {
	mod *pgxpool.Pool
	app *pgxpool.Pool
	log *slog.Logger
}

func NewStore(mod, app *pgxpool.Pool, log *slog.Logger) *Store {
	if log == nil {
		log = slog.Default()
	}
	return &Store{mod: mod, app: app, log: log}
}

const caseColumns = `c.id, c.kind, c.user_id, c.pack_kind, c.pack_id, c.from_status, c.to_status, c.opened_at, c.deadline,
       c.state, c.decided_at, s.username, c.reason_code, c.reason_text, c.request_text`

func scanCase(row pgx.CollectableRow) (Case, error) {
	var c Case
	err := row.Scan(&c.ID, &c.Kind, &c.UserID, &c.PackKind, &c.PackID, &c.FromStatus, &c.ToStatus, &c.OpenedAt, &c.Deadline,
		&c.State, &c.DecidedAt, &c.DecidedBy, &c.ReasonCode, &c.ReasonText, &c.RequestText)
	return c, err
}

func (s *Store) List(ctx context.Context, open bool, kind string, limit int) ([]Case, error) {
	query := `SELECT ` + caseColumns + ` FROM cases c LEFT JOIN staff_accounts s ON s.id = c.decided_by
WHERE c.state = 'open' AND ($2 = '' OR c.kind = $2) ORDER BY c.deadline, c.id LIMIT $1`
	if !open {
		query = `SELECT ` + caseColumns + ` FROM cases c LEFT JOIN staff_accounts s ON s.id = c.decided_by
WHERE c.state <> 'open' AND ($2 = '' OR c.kind = $2) ORDER BY c.decided_at DESC, c.id DESC LIMIT $1`
	}
	rows, err := s.mod.Query(ctx, query, limit, kind)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, scanCase)
}

func (s *Store) Get(ctx context.Context, id int64) (Case, error) {
	rows, err := s.mod.Query(ctx, `SELECT `+caseColumns+` FROM cases c LEFT JOIN staff_accounts s ON s.id = c.decided_by WHERE c.id = $1`, id)
	if err != nil {
		return Case{}, err
	}
	c, err := pgx.CollectExactlyOneRow(rows, scanCase)
	if errors.Is(err, pgx.ErrNoRows) {
		return Case{}, ErrNotFound
	}
	return c, err
}

type Decision struct {
	StaffID    int64
	ReasonCode string
	ReasonText string
	At         time.Time
}

func CloseUser(ctx context.Context, tx pgx.Tx, userID int64, newStatus string, d Decision) ([]int64, error) {
	rows, err := tx.Query(ctx, `UPDATE cases SET
    state = CASE WHEN $2 >= (CASE to_status WHEN 'suspended' THEN 4 WHEN 'restricted' THEN 3 WHEN 'limited' THEN 2 WHEN 'warned' THEN 1 ELSE 0 END)
                 THEN 'confirmed' ELSE 'reversed' END,
    decided_at = $3, decided_by = $4, reason_code = $5, reason_text = $6
WHERE state = 'open' AND kind IN ('account_status', 'review_request') AND user_id = $1
RETURNING id`, userID, Level(newStatus), d.At, d.StaffID, d.ReasonCode, d.ReasonText)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[int64])
}

func ClosePack(ctx context.Context, tx pgx.Tx, kind string, packID int64, state string, d Decision) ([]int64, error) {
	rows, err := tx.Query(ctx, `UPDATE cases SET state = $3, decided_at = $4, decided_by = $5, reason_code = $6, reason_text = $7
WHERE state = 'open' AND kind = 'pack_hidden' AND pack_kind = $1 AND pack_id = $2
RETURNING id`, kind, packID, state, d.At, d.StaffID, d.ReasonCode, d.ReasonText)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[int64])
}

func (s *Store) Scan(ctx context.Context) (int, error) {
	if s.app == nil {
		return 0, nil
	}
	accounts, err := s.scanStatusEvents(ctx)
	if err != nil {
		return accounts, fmt.Errorf("status events: %w", err)
	}
	packs, err := s.scanHiddenPacks(ctx)
	if err != nil {
		return accounts + packs, fmt.Errorf("hidden packs: %w", err)
	}
	requests, err := s.scanReviewRequests(ctx)
	if err != nil {
		return accounts + packs + requests, fmt.Errorf("review requests: %w", err)
	}
	return accounts + packs + requests, nil
}

type statusEvent struct {
	ID         int64
	UserID     int64
	FromStatus string
	ToStatus   string
	Cause      string
	CreatedAt  time.Time
}

func (s *Store) scanStatusEvents(ctx context.Context) (int, error) {
	var last int64
	err := s.mod.QueryRow(ctx, `SELECT last_id FROM case_watch WHERE name = $1`, watchEvents).Scan(&last)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return 0, err
	}
	opened := 0
	for {
		rows, err := s.app.Query(ctx, `SELECT id, user_id, from_status, to_status, cause, created_at FROM user_moderation_events
WHERE id > $1 ORDER BY id LIMIT $2`, max(last-eventLag, 0), eventBatch)
		if err != nil {
			return opened, err
		}
		events, err := pgx.CollectRows(rows, pgx.RowToStructByPos[statusEvent])
		if err != nil {
			return opened, err
		}
		if len(events) == 0 {
			return opened, nil
		}
		n, top, err := s.openAccountCases(ctx, events, last)
		opened += n
		if err != nil {
			return opened, err
		}
		if top <= last || len(events) < eventBatch {
			return opened, nil
		}
		last = top
	}
}

func heavyRaise(e statusEvent) bool {
	return Level(e.ToStatus) >= Level("restricted") && Level(e.ToStatus) > Level(e.FromStatus)
}

func (s *Store) openAccountCases(ctx context.Context, events []statusEvent, last int64) (int, int64, error) {
	tx, err := s.mod.Begin(ctx)
	if err != nil {
		return 0, last, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	opened := 0
	top := last
	for _, e := range events {
		top = max(top, e.ID)
		if !heavyRaise(e) || e.Cause == "manual" || time.Since(e.CreatedAt) > MaxAge {
			continue
		}
		var caseID int64
		err := tx.QueryRow(ctx, `INSERT INTO cases (kind, source_key, user_id, from_status, to_status, opened_at, deadline)
VALUES ('account_status', $1, $2, $3, $4, $5, $6) ON CONFLICT (source_key) DO NOTHING RETURNING id`,
			"status_event:"+strconv.FormatInt(e.ID, 10), e.UserID, e.FromStatus, e.ToStatus, e.CreatedAt, e.CreatedAt.Add(Deadline)).Scan(&caseID)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return opened, last, err
		}
		if _, err := tx.Exec(ctx, `UPDATE cases SET state = 'superseded', decided_at = $3
WHERE state = 'open' AND kind = 'account_status' AND user_id = $1 AND id <> $2`, e.UserID, caseID, e.CreatedAt); err != nil {
			return opened, last, err
		}
		opened++
	}
	if _, err := tx.Exec(ctx, `INSERT INTO case_watch (name, last_id) VALUES ($1, $2)
ON CONFLICT (name) DO UPDATE SET last_id = greatest(case_watch.last_id, EXCLUDED.last_id)`, watchEvents, top); err != nil {
		return opened, last, err
	}
	return opened, top, tx.Commit(ctx)
}

type hiddenPack struct {
	Kind     string
	ID       int64
	HiddenAt time.Time
}

func (s *Store) scanHiddenPacks(ctx context.Context) (int, error) {
	rows, err := s.app.Query(ctx, `SELECT 'sticker_pack', id, hidden_at FROM sticker_packs
WHERE hidden_by = 'reports' AND hidden_at > now() - interval '30 days' AND deleted_at IS NULL
UNION ALL
SELECT 'emoji_pack', id, hidden_at FROM emoji_packs
WHERE hidden_by = 'reports' AND hidden_at > now() - interval '30 days' AND deleted_at IS NULL`)
	if err != nil {
		return 0, err
	}
	packs, err := pgx.CollectRows(rows, pgx.RowToStructByPos[hiddenPack])
	if err != nil {
		return 0, err
	}
	opened := 0
	for _, p := range packs {
		key := fmt.Sprintf("%s:%d:%d", p.Kind, p.ID, p.HiddenAt.UnixMicro())
		tag, err := s.mod.Exec(ctx, `INSERT INTO cases (kind, source_key, pack_kind, pack_id, opened_at, deadline)
VALUES ('pack_hidden', $1, $2, $3, $4, $5) ON CONFLICT (source_key) DO NOTHING`, key, p.Kind, p.ID, p.HiddenAt, p.HiddenAt.Add(Deadline))
		if err != nil {
			return opened, err
		}
		opened += int(tag.RowsAffected())
	}
	return opened, nil
}

func (s *Store) Run(ctx context.Context, every time.Duration) {
	scan := func() {
		n, err := s.Scan(ctx)
		if err != nil && ctx.Err() == nil {
			s.log.ErrorContext(ctx, "case watcher failed", "error", err)
			return
		}
		if n > 0 {
			s.log.InfoContext(ctx, "cases opened", "count", n)
		}
	}
	scan()
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			scan()
		}
	}
}

func (s *Store) ForPack(ctx context.Context, kind string, packID int64) ([]Case, error) {
	rows, err := s.mod.Query(ctx, `SELECT `+caseColumns+` FROM cases c LEFT JOIN staff_accounts s ON s.id = c.decided_by
WHERE c.kind = 'pack_hidden' AND c.pack_kind = $1 AND c.pack_id = $2 ORDER BY c.opened_at DESC, c.id DESC LIMIT 50`, kind, packID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, scanCase)
}

func (s *Store) ForUser(ctx context.Context, userID int64) ([]Case, error) {
	rows, err := s.mod.Query(ctx, `SELECT `+caseColumns+` FROM cases c LEFT JOIN staff_accounts s ON s.id = c.decided_by
WHERE c.user_id = $1 ORDER BY c.opened_at DESC, c.id DESC LIMIT 50`, userID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, scanCase)
}
