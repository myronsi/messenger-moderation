package cases

import (
	"context"
	"time"
)

type Notice struct {
	NotifierName  string     `json:"notifier_name"`
	NotifierEmail *string    `json:"notifier_email"`
	Content       string     `json:"content"`
	Location      string     `json:"location"`
	Explanation   string     `json:"explanation"`
	ReceivedAt    time.Time  `json:"received_at"`
	EnteredBy     string     `json:"entered_by"`
	Outcome       *string    `json:"outcome"`
	MailState     string     `json:"mail_state"`
	MailSentAt    *time.Time `json:"mail_sent_at"`
}

func (s *Store) Notices(ctx context.Context, caseIDs []int64) (map[int64]Notice, error) {
	out := map[int64]Notice{}
	if len(caseIDs) == 0 {
		return out, nil
	}
	rows, err := s.mod.Query(ctx, `SELECT n.case_id, n.notifier_name, n.notifier_email, n.content, n.location, n.explanation,
       n.received_at, s.username, n.outcome, n.mail_state, n.mail_sent_at
FROM notices n JOIN staff_accounts s ON s.id = n.entered_by WHERE n.case_id = ANY($1)`, caseIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var n Notice
		if err := rows.Scan(&id, &n.NotifierName, &n.NotifierEmail, &n.Content, &n.Location, &n.Explanation,
			&n.ReceivedAt, &n.EnteredBy, &n.Outcome, &n.MailState, &n.MailSentAt); err != nil {
			return nil, err
		}
		out[id] = n
	}
	return out, rows.Err()
}
