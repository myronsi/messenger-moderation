package review

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("account not found")

type Review struct {
	UserID int64
	At     time.Time
}

type Cursor struct {
	Level    int       `json:"l"`
	Weighted time.Time `json:"w"`
	Score    float64   `json:"s"`
	UserID   int64     `json:"u"`
}

type QueueQuery struct {
	Statuses     []string
	Reasons      []string
	OpenedAfter  *time.Time
	OpenedBefore *time.Time
	After        *Cursor
	Limit        int
	Reviews      []Review
}

type UserSummary struct {
	ID          int64     `json:"id,string"`
	Username    string    `json:"username"`
	DisplayName string    `json:"display_name"`
	CreatedAt   time.Time `json:"created_at"`
}

type QueueItem struct {
	User            UserSummary `json:"user"`
	Status          string      `json:"status"`
	Score           float64     `json:"score"`
	StatusSince     *time.Time  `json:"status_since"`
	OpenReports     int         `json:"open_reports"`
	WeightedReports int         `json:"weighted_reports"`
	Reasons         []string    `json:"reasons"`
	NewestReportAt  *time.Time  `json:"newest_report_at"`
	OpenedAt        time.Time   `json:"opened_at"`
	cursor          Cursor
}

type Profile struct {
	UserSummary
	Bio        *string   `json:"bio"`
	LastSeenAt time.Time `json:"last_seen_at"`
}

type CurrentStatus struct {
	Status             string     `json:"status"`
	Score              float64    `json:"score"`
	Since              *time.Time `json:"since"`
	ProbationUntil     *time.Time `json:"probation_until"`
	AppealBlockedUntil *time.Time `json:"appeal_blocked_until"`
	LastAppealAt       *time.Time `json:"last_appeal_at"`
}

type StatusChange struct {
	ID         int64     `json:"id,string"`
	FromStatus string    `json:"from_status"`
	ToStatus   string    `json:"to_status"`
	Score      float64   `json:"score"`
	Cause      string    `json:"cause"`
	CreatedAt  time.Time `json:"created_at"`
}

type Reporter struct {
	UserSummary
	Status string `json:"status"`
}

type Report struct {
	Reporter    Reporter        `json:"reporter"`
	Reason      string          `json:"reason"`
	Note        *string         `json:"note"`
	Context     string          `json:"context"`
	Weight      float64         `json:"weight"`
	Snapshot    json.RawMessage `json:"snapshot"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
	WithdrawnAt *time.Time      `json:"withdrawn_at"`
}

type Account struct {
	User    Profile        `json:"user"`
	Status  CurrentStatus  `json:"status"`
	History []StatusChange `json:"history"`
	Reports []Report       `json:"reports"`
}

type UserHit struct {
	UserSummary
	Status string `json:"status"`
}

type Store struct {
	db *pgxpool.Pool
}

func NewStore(db *pgxpool.Pool) *Store { return &Store{db: db} }

const queueSQL = `
WITH reviews AS (
    SELECT * FROM unnest($1::bigint[], $2::timestamptz[]) AS v (user_id, reviewed_at)
), open_reports AS (
    SELECT r.target_id AS user_id,
           count(*)::int AS open_reports,
           count(*) FILTER (WHERE r.weight > 0)::int AS weighted_reports,
           min(r.created_at) AS oldest_report_at,
           max(r.created_at) AS newest_report_at,
           max(r.created_at) FILTER (WHERE r.weight > 0) AS newest_weighted_at,
           array_agg(DISTINCT r.reason ORDER BY r.reason)::text[] AS reasons
    FROM user_reports r
    LEFT JOIN reviews v ON v.user_id = r.target_id
    WHERE r.withdrawn_at IS NULL AND (v.reviewed_at IS NULL OR r.created_at > v.reviewed_at)
    GROUP BY r.target_id
), heavy AS (
    SELECT m.user_id, m.status_since
    FROM user_moderation m
    LEFT JOIN reviews v ON v.user_id = m.user_id
    WHERE m.status IN ('restricted', 'suspended') AND (v.reviewed_at IS NULL OR m.status_since > v.reviewed_at)
), items AS (
    SELECT u.id AS user_id, u.username, u.display_name, u.created_at AS account_created_at,
           coalesce(m.status, 'good') AS status,
           coalesce(m.score, 0)::float8 AS score,
           CASE WHEN m.status_since > 'epoch' THEN m.status_since END AS status_since,
           coalesce(o.open_reports, 0) AS open_reports,
           coalesce(o.weighted_reports, 0) AS weighted_reports,
           o.newest_report_at,
           coalesce(o.reasons, '{}')::text[] AS reasons,
           least(o.oldest_report_at, h.status_since) AS opened_at,
           CASE coalesce(m.status, 'good')
               WHEN 'suspended' THEN 4 WHEN 'restricted' THEN 3 WHEN 'limited' THEN 2 WHEN 'warned' THEN 1 ELSE 0
           END AS level,
           coalesce(o.newest_weighted_at, 'epoch') AS weighted_key
    FROM (SELECT user_id FROM open_reports UNION SELECT user_id FROM heavy) q
    JOIN users u ON u.id = q.user_id
    LEFT JOIN user_moderation m ON m.user_id = q.user_id
    LEFT JOIN open_reports o ON o.user_id = q.user_id
    LEFT JOIN heavy h ON h.user_id = q.user_id
)
SELECT user_id, username, display_name, account_created_at, status, score, status_since, open_reports,
       weighted_reports, newest_report_at, reasons, opened_at, level, weighted_key
FROM items
WHERE ($3::text[] IS NULL OR status = ANY ($3::text[]))
  AND ($4::text[] IS NULL OR reasons && $4::text[])
  AND ($5::timestamptz IS NULL OR opened_at >= $5::timestamptz)
  AND ($6::timestamptz IS NULL OR opened_at < $6::timestamptz)
  AND ($7::int IS NULL OR (level, weighted_key, score, user_id) < ($7::int, $8::timestamptz, $9::float8, $10::bigint))
ORDER BY level DESC, weighted_key DESC, score DESC, user_id DESC
LIMIT $11`

func (s *Store) Queue(ctx context.Context, q QueueQuery) ([]QueueItem, *Cursor, error) {
	ids := make([]int64, len(q.Reviews))
	times := make([]time.Time, len(q.Reviews))
	for i, r := range q.Reviews {
		ids[i], times[i] = r.UserID, r.At
	}
	var level *int
	var weighted *time.Time
	var score *float64
	var after *int64
	if q.After != nil {
		level, weighted, score, after = &q.After.Level, &q.After.Weighted, &q.After.Score, &q.After.UserID
	}
	rows, err := s.db.Query(ctx, queueSQL, ids, times, nilIfEmpty(q.Statuses), nilIfEmpty(q.Reasons),
		q.OpenedAfter, q.OpenedBefore, level, weighted, score, after, q.Limit+1)
	if err != nil {
		return nil, nil, err
	}
	items, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (QueueItem, error) {
		var it QueueItem
		err := row.Scan(&it.User.ID, &it.User.Username, &it.User.DisplayName, &it.User.CreatedAt, &it.Status, &it.Score,
			&it.StatusSince, &it.OpenReports, &it.WeightedReports, &it.NewestReportAt, &it.Reasons, &it.OpenedAt,
			&it.cursor.Level, &it.cursor.Weighted)
		it.cursor.Score, it.cursor.UserID = it.Score, it.User.ID
		return it, err
	})
	if err != nil {
		return nil, nil, err
	}
	if len(items) <= q.Limit {
		return items, nil, nil
	}
	items = items[:q.Limit]
	next := items[len(items)-1].cursor
	return items, &next, nil
}

func nilIfEmpty(v []string) []string {
	if len(v) == 0 {
		return nil
	}
	return v
}

func (s *Store) Account(ctx context.Context, userID int64) (Account, error) {
	var a Account
	err := pgx.BeginTxFunc(ctx, s.db, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
SELECT u.id, u.username, u.display_name, u.created_at, u.bio, u.last_seen_at,
       coalesce(m.status, 'good'), coalesce(m.score, 0)::float8,
       CASE WHEN m.status_since > 'epoch' THEN m.status_since END,
       m.probation_until, m.appeal_blocked_until, m.last_appeal_at
FROM users u LEFT JOIN user_moderation m ON m.user_id = u.id
WHERE u.id = $1`, userID).Scan(&a.User.ID, &a.User.Username, &a.User.DisplayName, &a.User.CreatedAt, &a.User.Bio,
			&a.User.LastSeenAt, &a.Status.Status, &a.Status.Score, &a.Status.Since, &a.Status.ProbationUntil,
			&a.Status.AppealBlockedUntil, &a.Status.LastAppealAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
SELECT id, from_status, to_status, score::float8, cause, created_at
FROM user_moderation_events WHERE user_id = $1 ORDER BY created_at DESC, id DESC`, userID)
		if err != nil {
			return err
		}
		a.History, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (StatusChange, error) {
			var c StatusChange
			err := row.Scan(&c.ID, &c.FromStatus, &c.ToStatus, &c.Score, &c.Cause, &c.CreatedAt)
			return c, err
		})
		if err != nil {
			return err
		}
		rows, err = tx.Query(ctx, `
SELECT r.reporter_id, ru.username, ru.display_name, ru.created_at, coalesce(rm.status, 'good'),
       r.reason, r.note, r.context, r.weight::float8, r.snapshot, r.created_at, r.updated_at, r.withdrawn_at
FROM user_reports r
JOIN users ru ON ru.id = r.reporter_id
LEFT JOIN user_moderation rm ON rm.user_id = r.reporter_id
WHERE r.target_id = $1
ORDER BY r.created_at DESC, r.reporter_id`, userID)
		if err != nil {
			return err
		}
		a.Reports, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (Report, error) {
			var r Report
			var snapshot []byte
			err := row.Scan(&r.Reporter.ID, &r.Reporter.Username, &r.Reporter.DisplayName, &r.Reporter.CreatedAt,
				&r.Reporter.Status, &r.Reason, &r.Note, &r.Context, &r.Weight, &snapshot, &r.CreatedAt, &r.UpdatedAt, &r.WithdrawnAt)
			r.Snapshot = snapshot
			return r, err
		})
		return err
	})
	return a, err
}

const searchSQL = `
SELECT u.id, u.username, u.display_name, u.created_at, coalesce(m.status, 'good')
FROM users u LEFT JOIN user_moderation m ON m.user_id = u.id
WHERE u.id = $3
   OR lower(u.username) LIKE $1 || '%'
   OR lower(u.display_name) LIKE '%' || $1 || '%'
ORDER BY (u.id = $3) DESC, (lower(u.username) = $2) DESC, (lower(u.username) LIKE $1 || '%') DESC, u.username, u.id
LIMIT $4`

func (s *Store) Search(ctx context.Context, term string, limit int) ([]UserHit, error) {
	term = strings.ToLower(term)
	id, err := strconv.ParseInt(term, 10, 64)
	if err != nil {
		id = 0
	}
	rows, err := s.db.Query(ctx, searchSQL, escapeLike(term), term, id, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (UserHit, error) {
		var h UserHit
		err := row.Scan(&h.ID, &h.Username, &h.DisplayName, &h.CreatedAt, &h.Status)
		return h, err
	})
}

var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

func escapeLike(s string) string { return likeEscaper.Replace(s) }
