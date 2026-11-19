package audit

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5/pgconn"
)

type Execer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

type Entry struct {
	StaffID    *int64
	Action     string
	TargetType string
	TargetID   string
	Details    map[string]any
}

func Write(ctx context.Context, db Execer, e Entry) error {
	details := e.Details
	if details == nil {
		details = map[string]any{}
	}
	raw, err := json.Marshal(details)
	if err != nil {
		return err
	}
	_, err = db.Exec(ctx,
		`INSERT INTO audit_log (staff_id, action, target_type, target_id, details) VALUES ($1, $2, NULLIF($3, ''), NULLIF($4, ''), $5)`,
		e.StaffID, e.Action, e.TargetType, e.TargetID, raw)
	return err
}
