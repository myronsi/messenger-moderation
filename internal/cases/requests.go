package cases

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type reviewRequest struct {
	ID        int64
	UserID    int64
	Status    string
	Text      string
	CreatedAt time.Time
}

func missingTable(err error) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == "42P01"
}

func (s *Store) scanReviewRequests(ctx context.Context) (int, error) {
	var last int64
	err := s.mod.QueryRow(ctx, `SELECT last_id FROM case_watch WHERE name = $1`, watchRequests).Scan(&last)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return 0, err
	}
	opened := 0
	for {
		rows, err := s.app.Query(ctx, `SELECT id, user_id, status, text, created_at FROM account_review_requests
WHERE id > $1 AND closed_at IS NULL ORDER BY id LIMIT $2`, max(last-eventLag, 0), eventBatch)
		if missingTable(err) {
			return opened, nil
		}
		if err != nil {
			return opened, err
		}
		requests, err := pgx.CollectRows(rows, pgx.RowToStructByPos[reviewRequest])
		if err != nil {
			return opened, err
		}
		n, top, err := s.openRequestCases(ctx, requests, last)
		opened += n
		if err != nil {
			return opened, err
		}
		if len(requests) < eventBatch || top <= last {
			break
		}
		last = top
	}
	return opened, s.settleRequests(ctx)
}

func (s *Store) openRequestCases(ctx context.Context, requests []reviewRequest, last int64) (int, int64, error) {
	tx, err := s.mod.Begin(ctx)
	if err != nil {
		return 0, last, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	opened := 0
	top := last
	for _, r := range requests {
		top = max(top, r.ID)
		tag, err := tx.Exec(ctx, `INSERT INTO cases (kind, source_key, user_id, to_status, request_text, opened_at, deadline)
VALUES ('review_request', $1, $2, $3, $4, $5, $6) ON CONFLICT (source_key) DO NOTHING`,
			requestKey(r.ID), r.UserID, r.Status, r.Text, r.CreatedAt, r.CreatedAt.Add(Deadline))
		if err != nil {
			return opened, last, err
		}
		opened += int(tag.RowsAffected())
	}
	if _, err := tx.Exec(ctx, `INSERT INTO case_watch (name, last_id) VALUES ($1, $2)
ON CONFLICT (name) DO UPDATE SET last_id = greatest(case_watch.last_id, EXCLUDED.last_id)`, watchRequests, top); err != nil {
		return opened, last, err
	}
	return opened, top, tx.Commit(ctx)
}

func requestKey(id int64) string { return "review_request:" + strconv.FormatInt(id, 10) }

func (s *Store) settleRequests(ctx context.Context) error {
	rows, err := s.mod.Query(ctx, `SELECT id, source_key FROM cases WHERE state = 'open' AND kind = 'review_request'`)
	if err != nil {
		return err
	}
	open := map[int64]int64{}
	var requestIDs []int64
	for rows.Next() {
		var caseID int64
		var key string
		if err := rows.Scan(&caseID, &key); err != nil {
			rows.Close()
			return err
		}
		requestID, err := strconv.ParseInt(key[len("review_request:"):], 10, 64)
		if err != nil {
			continue
		}
		open[requestID] = caseID
		requestIDs = append(requestIDs, requestID)
	}
	rows.Close()
	if err := rows.Err(); err != nil || len(requestIDs) == 0 {
		return err
	}
	closedRows, err := s.app.Query(ctx, `SELECT id, closed_at FROM account_review_requests
WHERE id = ANY($1) AND close_reason = 'status_good'`, requestIDs)
	if err != nil {
		return err
	}
	type closed struct {
		ID       int64
		ClosedAt time.Time
	}
	settled, err := pgx.CollectRows(closedRows, pgx.RowToStructByPos[closed])
	if err != nil {
		return err
	}
	for _, c := range settled {
		if _, err := s.mod.Exec(ctx, `UPDATE cases SET state = 'superseded', decided_at = $2 WHERE id = $1 AND state = 'open'`, open[c.ID], c.ClosedAt); err != nil {
			return err
		}
	}
	return nil
}
