package review

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/myronsi/messenger-moderation/internal/audit"
)

const (
	DefaultQueueLimit = 50
	MaxQueueLimit     = 100
	SearchLimit       = 20
	minSearchLength   = 2
	maxSearchLength   = 64
)

var (
	ErrUnavailable = errors.New("messenger data is not configured")
	ErrInvalid     = errors.New("invalid request")
)

var (
	Statuses = []string{"good", "warned", "limited", "restricted", "suspended"}
	Reasons  = []string{"spam", "scam", "harassment", "impersonation", "inappropriate_profile", "other"}
)

type ReviewSource func(ctx context.Context) ([]Review, error)

type Service struct {
	store   *Store
	audit   audit.Execer
	reviews ReviewSource
}

func NewService(store *Store, auditDB audit.Execer, reviews ReviewSource) *Service {
	return &Service{store: store, audit: auditDB, reviews: reviews}
}

type QueuePage struct {
	Items      []QueueItem `json:"items"`
	NextCursor *string     `json:"next_cursor"`
}

func (s *Service) Queue(ctx context.Context, staffID int64, q QueueQuery) (QueuePage, error) {
	if s.store == nil {
		return QueuePage{}, ErrUnavailable
	}
	if q.Limit <= 0 {
		q.Limit = DefaultQueueLimit
	}
	if q.Limit > MaxQueueLimit {
		q.Limit = MaxQueueLimit
	}
	if s.reviews != nil {
		reviews, err := s.reviews(ctx)
		if err != nil {
			return QueuePage{}, fmt.Errorf("staff reviews: %w", err)
		}
		q.Reviews = reviews
	}
	items, next, err := s.store.Queue(ctx, q)
	if err != nil {
		return QueuePage{}, err
	}
	page := QueuePage{Items: items}
	if next != nil {
		c := EncodeCursor(*next)
		page.NextCursor = &c
	}
	if err := audit.Write(ctx, s.audit, audit.Entry{
		StaffID: &staffID, Action: "queue.viewed",
		Details: map[string]any{"filters": queueFilters(q), "user_ids": queueUserIDs(items)},
	}); err != nil {
		return QueuePage{}, fmt.Errorf("data access log: %w", err)
	}
	return page, nil
}

func queueFilters(q QueueQuery) map[string]any {
	f := map[string]any{}
	if len(q.Statuses) > 0 {
		f["status"] = q.Statuses
	}
	if len(q.Reasons) > 0 {
		f["reason"] = q.Reasons
	}
	if q.OpenedAfter != nil {
		f["opened_after"] = q.OpenedAfter.UTC().Format(time.RFC3339)
	}
	if q.OpenedBefore != nil {
		f["opened_before"] = q.OpenedBefore.UTC().Format(time.RFC3339)
	}
	if q.After != nil {
		f["page"] = "next"
	}
	return f
}

func queueUserIDs(items []QueueItem) []string {
	ids := make([]string, len(items))
	for i, it := range items {
		ids[i] = strconv.FormatInt(it.User.ID, 10)
	}
	return ids
}

func (s *Service) Account(ctx context.Context, staffID, userID int64) (Account, error) {
	if s.store == nil {
		return Account{}, ErrUnavailable
	}
	a, err := s.store.Account(ctx, userID)
	if err != nil {
		return Account{}, err
	}
	if err := audit.Write(ctx, s.audit, audit.Entry{
		StaffID: &staffID, Action: "user.viewed", TargetType: "user", TargetID: strconv.FormatInt(userID, 10),
		Details: map[string]any{"reports": len(a.Reports), "status_changes": len(a.History)},
	}); err != nil {
		return Account{}, fmt.Errorf("data access log: %w", err)
	}
	return a, nil
}

func SearchTerm(raw string) (string, error) {
	term := strings.TrimPrefix(strings.TrimSpace(raw), "@")
	switch {
	case utf8.RuneCountInString(term) > maxSearchLength:
		return "", fmt.Errorf("%w: the search is longer than %d characters", ErrInvalid, maxSearchLength)
	case utf8.RuneCountInString(term) < minSearchLength && !isDigits(term):
		return "", fmt.Errorf("%w: type at least %d characters or an ID", ErrInvalid, minSearchLength)
	}
	return term, nil
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func (s *Service) Search(ctx context.Context, staffID int64, raw string) ([]UserHit, error) {
	if s.store == nil {
		return nil, ErrUnavailable
	}
	term, err := SearchTerm(raw)
	if err != nil {
		return nil, err
	}
	hits, err := s.store.Search(ctx, term, SearchLimit)
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(hits))
	for i, h := range hits {
		ids[i] = strconv.FormatInt(h.ID, 10)
	}
	if err := audit.Write(ctx, s.audit, audit.Entry{
		StaffID: &staffID, Action: "users.searched",
		Details: map[string]any{"query": term, "user_ids": ids},
	}); err != nil {
		return nil, fmt.Errorf("data access log: %w", err)
	}
	return hits, nil
}
