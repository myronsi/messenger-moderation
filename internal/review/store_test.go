package review_test

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/myronsi/messenger-moderation/internal/appdb/appdbtest"
	"github.com/myronsi/messenger-moderation/internal/review"
)

var now = time.Now().UTC().Truncate(time.Second)

type seed struct {
	t  *testing.T
	db appdbtest.DB
}

func newSeed(t *testing.T) seed { return seed{t: t, db: appdbtest.New(t)} }

func (s seed) exec(sql string, args ...any) {
	s.t.Helper()
	if _, err := s.db.Write.Exec(context.Background(), sql, args...); err != nil {
		s.t.Fatalf("%s: %v", sql, err)
	}
}

func (s seed) user(id int64, username, name string) {
	s.t.Helper()
	s.exec(`INSERT INTO users (id, username, display_name, created_at) VALUES ($1, $2, $3, $4)`, id, username, name, now.Add(-90*24*time.Hour))
}

func (s seed) report(reporter, target int64, reason string, weight float64, ago time.Duration) {
	s.t.Helper()
	s.exec(`INSERT INTO user_reports (reporter_id, target_id, reason, context, weight, created_at, updated_at, snapshot)
VALUES ($1, $2, $3, 'none', $4, $5, $5, '{"username":"old_name","display_name":"Old","bio":null}')`, reporter, target, reason, weight, now.Add(-ago))
}

func (s seed) status(user int64, status string, score float64, ago time.Duration) {
	s.t.Helper()
	s.exec(`INSERT INTO user_moderation (user_id, status, score, status_since) VALUES ($1, $2, $3, $4)`, user, status, score, now.Add(-ago))
}

func usernames(items []review.QueueItem) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.User.Username
	}
	return out
}

func standardQueue(t *testing.T) (seed, *review.Store) {
	s := newSeed(t)
	for i, name := range []string{"alice", "bob", "carol", "dave", "erin", "frank", "grace", "heidi", "ivan"} {
		s.user(int64(i+1), name, name)
	}
	s.status(7, "suspended", 16, 5*24*time.Hour)
	s.status(4, "warned", 3.5, 2*time.Hour)
	s.status(8, "restricted", 11, 3*24*time.Hour)
	s.status(2, "good", 0, 0)
	s.exec(`UPDATE user_moderation SET status_since = 'epoch' WHERE user_id = 2`)
	s.report(1, 4, "spam", 1, 2*time.Hour)
	s.report(2, 4, "scam", 0.25, 3*24*time.Hour)
	s.report(3, 5, "harassment", 0.5, 10*24*time.Hour)
	s.report(1, 6, "other", 0, time.Hour)
	s.report(2, 3, "spam", 0.25, 24*time.Hour)
	s.exec(`UPDATE user_reports SET withdrawn_at = now() WHERE target_id = 3`)
	return s, review.NewStore(s.db.Read)
}

func TestQueueListsAccountsByUrgency(t *testing.T) {
	_, store := standardQueue(t)
	items, next, err := store.Queue(context.Background(), review.QueueQuery{Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := usernames(items), []string{"grace", "heidi", "dave", "erin", "frank"}; !slices.Equal(got, want) {
		t.Fatalf("queue = %v, want %v", got, want)
	}
	if next != nil {
		t.Fatalf("next cursor on the last page: %+v", next)
	}
	grace, dave, frank := items[0], items[2], items[4]
	if grace.OpenReports != 0 || grace.NewestReportAt != nil || !grace.OpenedAt.Equal(now.Add(-5*24*time.Hour)) || grace.Score != 16 {
		t.Fatalf("grace = %+v", grace)
	}
	if dave.OpenReports != 2 || dave.WeightedReports != 2 || !slices.Equal(dave.Reasons, []string{"scam", "spam"}) ||
		!dave.OpenedAt.Equal(now.Add(-3*24*time.Hour)) || !dave.NewestReportAt.Equal(now.Add(-2*time.Hour)) || dave.StatusSince == nil {
		t.Fatalf("dave = %+v", dave)
	}
	if frank.Status != "good" || frank.WeightedReports != 0 || frank.StatusSince != nil {
		t.Fatalf("frank = %+v", frank)
	}
}

func TestQueueFilters(t *testing.T) {
	_, store := standardQueue(t)
	ctx := context.Background()
	after, before := now.Add(-7*24*time.Hour), now.Add(-24*time.Hour)
	cases := []struct {
		name string
		q    review.QueueQuery
		want []string
	}{
		{"status", review.QueueQuery{Statuses: []string{"good", "warned"}}, []string{"dave", "erin", "frank"}},
		{"reason", review.QueueQuery{Reasons: []string{"scam", "other"}}, []string{"dave", "frank"}},
		{"newer than a day", review.QueueQuery{OpenedAfter: &before}, []string{"frank"}},
		{"one to seven days", review.QueueQuery{OpenedAfter: &after, OpenedBefore: &before}, []string{"grace", "heidi", "dave"}},
		{"older than a week", review.QueueQuery{OpenedBefore: &after}, []string{"erin"}},
	}
	for _, c := range cases {
		c.q.Limit = 50
		items, _, err := store.Queue(ctx, c.q)
		if err != nil {
			t.Fatal(err)
		}
		if got := usernames(items); !slices.Equal(got, c.want) {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

func TestQueueSkipsWhatStaffAlreadyReviewed(t *testing.T) {
	s, store := standardQueue(t)
	reviews := []review.Review{
		{UserID: 4, At: now.Add(-24 * time.Hour)},
		{UserID: 7, At: now.Add(-time.Hour)},
		{UserID: 5, At: now.Add(-time.Minute)},
	}
	items, _, err := store.Queue(context.Background(), review.QueueQuery{Limit: 50, Reviews: reviews})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := usernames(items), []string{"heidi", "dave", "frank"}; !slices.Equal(got, want) {
		t.Fatalf("queue = %v, want %v", got, want)
	}
	if items[1].OpenReports != 1 || !slices.Equal(items[1].Reasons, []string{"spam"}) {
		t.Fatalf("dave after review = %+v", items[1])
	}

	s.exec(`UPDATE user_moderation SET status = 'suspended', status_since = $1 WHERE user_id = 7`, now)
	items, _, err = store.Queue(context.Background(), review.QueueQuery{Limit: 50, Reviews: reviews})
	if err != nil {
		t.Fatal(err)
	}
	if items[0].User.Username != "grace" {
		t.Fatalf("a status set after the review must come back: %v", usernames(items))
	}
}

func TestQueuePagesWithTheCursor(t *testing.T) {
	_, store := standardQueue(t)
	var seen []string
	var cursor *review.Cursor
	for range 5 {
		items, next, err := store.Queue(context.Background(), review.QueueQuery{Limit: 2, After: cursor})
		if err != nil {
			t.Fatal(err)
		}
		seen = append(seen, usernames(items)...)
		if next == nil {
			break
		}
		decoded, err := review.DecodeCursor(review.EncodeCursor(*next))
		if err != nil {
			t.Fatal(err)
		}
		cursor = &decoded
	}
	if want := []string{"grace", "heidi", "dave", "erin", "frank"}; !slices.Equal(seen, want) {
		t.Fatalf("pages = %v, want %v", seen, want)
	}
}

func TestAccountShowsStatusHistoryAndReports(t *testing.T) {
	s, store := standardQueue(t)
	s.exec(`UPDATE users SET bio = 'Cheap watches', last_seen_at = $1 WHERE id = 4`, now)
	s.exec(`UPDATE user_moderation SET probation_until = $1, last_appeal_at = $2 WHERE user_id = 4`, now.Add(14*24*time.Hour), now.Add(-time.Hour))
	s.exec(`INSERT INTO user_moderation_events (user_id, from_status, to_status, score, cause, created_at) VALUES
(4, 'good', 'limited', 6, 'reports', $1), (4, 'limited', 'warned', 3.5, 'appeal', $2)`, now.Add(-3*time.Hour), now.Add(-time.Hour))
	s.exec(`UPDATE user_reports SET withdrawn_at = $1, note = 'sorry' WHERE reporter_id = 2 AND target_id = 4`, now)

	a, err := store.Account(context.Background(), 4)
	if err != nil {
		t.Fatal(err)
	}
	if a.User.Username != "dave" || a.User.Bio == nil || *a.User.Bio != "Cheap watches" || !a.User.LastSeenAt.Equal(now) {
		t.Fatalf("profile = %+v", a.User)
	}
	if a.Status.Status != "warned" || a.Status.Score != 3.5 || a.Status.ProbationUntil == nil || a.Status.LastAppealAt == nil || a.Status.AppealBlockedUntil != nil {
		t.Fatalf("status = %+v", a.Status)
	}
	if len(a.History) != 2 || a.History[0].Cause != "appeal" || a.History[1].ToStatus != "limited" {
		t.Fatalf("history = %+v", a.History)
	}
	if len(a.Reports) != 2 || a.Reports[0].Reporter.Username != "alice" || a.Reports[0].Weight != 1 || a.Reports[0].WithdrawnAt != nil {
		t.Fatalf("reports = %+v", a.Reports)
	}
	withdrawn := a.Reports[1]
	if withdrawn.Reporter.Status != "good" || withdrawn.WithdrawnAt == nil || withdrawn.Note == nil || *withdrawn.Note != "sorry" {
		t.Fatalf("withdrawn report = %+v", withdrawn)
	}
	var snap map[string]any
	if err := json.Unmarshal(withdrawn.Snapshot, &snap); err != nil || snap["username"] != "old_name" {
		t.Fatalf("snapshot = %s %v", withdrawn.Snapshot, err)
	}

	plain, err := store.Account(context.Background(), 9)
	if err != nil || plain.Status.Status != "good" || plain.Status.Since != nil || len(plain.History) != 0 || len(plain.Reports) != 0 {
		t.Fatalf("account without moderation rows = %+v, %v", plain, err)
	}
	if _, err := store.Account(context.Background(), 999); !errors.Is(err, review.ErrNotFound) {
		t.Fatalf("missing account error = %v", err)
	}
}

func TestSearchByUsernameNameOrID(t *testing.T) {
	s := newSeed(t)
	s.user(1, "dave", "Dave")
	s.user(2, "davina", "Davina K")
	s.user(3, "zed", "Old Dave Fan")
	s.user(4, "a_b", "Under Score")
	s.user(5, "axb", "Letter")
	s.status(1, "limited", 6, time.Hour)
	store := review.NewStore(s.db.Read)
	ctx := context.Background()

	names := func(term string) []string {
		t.Helper()
		hits, err := store.Search(ctx, term, 20)
		if err != nil {
			t.Fatal(err)
		}
		out := make([]string, len(hits))
		for i, h := range hits {
			out[i] = h.Username
		}
		return out
	}
	if got := names("DAVE"); !slices.Equal(got, []string{"dave", "zed"}) {
		t.Fatalf("dave = %v", got)
	}
	if got := names("dav"); !slices.Equal(got, []string{"dave", "davina", "zed"}) {
		t.Fatalf("dav = %v", got)
	}
	if got := names("a_"); !slices.Equal(got, []string{"a_b"}) {
		t.Fatalf("underscore must not be a wildcard: %v", got)
	}
	if got := names("5"); !slices.Equal(got, []string{"axb"}) {
		t.Fatalf("id = %v", got)
	}
	hits, _ := store.Search(ctx, "dave", 20)
	if hits[0].Status != "limited" || hits[1].Status != "good" {
		t.Fatalf("statuses = %+v", hits)
	}
}

func TestStoreConnectionIsReadOnly(t *testing.T) {
	s := newSeed(t)
	if _, err := s.db.Read.Exec(context.Background(), `INSERT INTO users (username, display_name) VALUES ('x_x', 'X')`); err == nil {
		t.Fatal("the Messenger connection accepted a write")
	}
}
