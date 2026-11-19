package decisions

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/myronsi/messenger-moderation/internal/audit"
	"github.com/myronsi/messenger-moderation/internal/cases"
	"github.com/myronsi/messenger-moderation/internal/review"
	"github.com/myronsi/messenger-moderation/internal/staff"
)

type Subject struct {
	ID          int64  `json:"id,string"`
	Username    string `json:"username,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
	Title       string `json:"title,omitempty"`
	Status      string `json:"status,omitempty"`
	Hidden      *bool  `json:"hidden,omitempty"`
}

type CaseView struct {
	cases.Case
	Subject *Subject `json:"subject"`
}

func (s *Service) Cases(ctx context.Context, st staff.Staff, open bool) ([]CaseView, error) {
	list, err := s.cases.List(ctx, open, CaseListLimit)
	if err != nil {
		return nil, err
	}
	subjects, err := s.subjects(ctx, list)
	if err != nil {
		return nil, err
	}
	out := make([]CaseView, len(list))
	ids := make([]string, len(list))
	for i, c := range list {
		out[i] = CaseView{Case: c, Subject: subjects[subjectKey(c)]}
		ids[i] = strconv.FormatInt(c.ID, 10)
	}
	state := "decided"
	if open {
		state = "open"
	}
	if err := audit.Write(ctx, s.mod, audit.Entry{StaffID: &st.ID, Action: "cases.viewed", Details: map[string]any{"state": state, "case_ids": ids}}); err != nil {
		return nil, fmt.Errorf("data access log: %w", err)
	}
	return out, nil
}

func subjectKey(c cases.Case) string {
	if c.UserID != nil {
		return "user:" + strconv.FormatInt(*c.UserID, 10)
	}
	return *c.PackKind + ":" + strconv.FormatInt(*c.PackID, 10)
}

func (s *Service) subjects(ctx context.Context, list []cases.Case) (map[string]*Subject, error) {
	out := map[string]*Subject{}
	if s.app == nil {
		return out, nil
	}
	var users, stickers, emoji []int64
	for _, c := range list {
		switch {
		case c.UserID != nil:
			users = append(users, *c.UserID)
		case *c.PackKind == "sticker_pack":
			stickers = append(stickers, *c.PackID)
		default:
			emoji = append(emoji, *c.PackID)
		}
	}
	rows, err := s.app.Query(ctx, `SELECT u.id, u.username, u.display_name, coalesce(m.status, 'good')
FROM users u LEFT JOIN user_moderation m ON m.user_id = u.id WHERE u.id = ANY($1)`, users)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var sub Subject
		if err := rows.Scan(&sub.ID, &sub.Username, &sub.DisplayName, &sub.Status); err != nil {
			rows.Close()
			return nil, err
		}
		out["user:"+strconv.FormatInt(sub.ID, 10)] = &sub
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for kind, ids := range map[string][]int64{"sticker_pack": stickers, "emoji_pack": emoji} {
		rows, err := s.app.Query(ctx, `SELECT id, title, hidden_at IS NOT NULL FROM `+packTable(kind)+` WHERE id = ANY($1)`, ids)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var sub Subject
			var hidden bool
			if err := rows.Scan(&sub.ID, &sub.Title, &hidden); err != nil {
				rows.Close()
				return nil, err
			}
			sub.Hidden = &hidden
			out[kind+":"+strconv.FormatInt(sub.ID, 10)] = &sub
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func packTable(kind string) string {
	if kind == "emoji_pack" {
		return "emoji_packs"
	}
	return "sticker_packs"
}

type PackItem struct {
	ID           int64  `json:"id,string"`
	AttachmentID string `json:"attachment_id"`
	Label        string `json:"label"`
	Animated     bool   `json:"animated"`
}

type PackReport struct {
	ReporterID       int64     `json:"reporter_id,string"`
	ReporterUsername string    `json:"reporter_username"`
	Reason           string    `json:"reason"`
	CreatedAt        time.Time `json:"created_at"`
}

type Pack struct {
	Kind      string              `json:"kind"`
	ID        int64               `json:"id,string"`
	Title     string              `json:"title"`
	Slug      *string             `json:"slug"`
	Owner     *review.UserSummary `json:"owner"`
	GroupPack bool                `json:"group_pack"`
	Builtin   bool                `json:"builtin"`
	HiddenAt  *time.Time          `json:"hidden_at"`
	HiddenBy  *string             `json:"hidden_by"`
	DeletedAt *time.Time          `json:"deleted_at"`
	CreatedAt time.Time           `json:"created_at"`
	Items     []PackItem          `json:"items"`
	Reports   []PackReport        `json:"reports"`
	Cases     []cases.Case        `json:"cases"`
}

func (s *Service) Pack(ctx context.Context, st staff.Staff, kind string, packID int64) (Pack, error) {
	if !slices.Contains(PackKinds, kind) {
		return Pack{}, fmt.Errorf("%w: unknown pack kind", ErrInvalid)
	}
	if s.app == nil {
		return Pack{}, ErrUnavailable
	}
	p, err := s.loadPack(ctx, kind, packID)
	if err != nil {
		return Pack{}, err
	}
	if p.Cases, err = s.cases.ForPack(ctx, kind, packID); err != nil {
		return Pack{}, err
	}
	if err := audit.Write(ctx, s.mod, audit.Entry{
		StaffID: &st.ID, Action: "pack.viewed", TargetType: kind, TargetID: strconv.FormatInt(packID, 10),
		Details: map[string]any{"items": len(p.Items), "reports": len(p.Reports)},
	}); err != nil {
		return Pack{}, fmt.Errorf("data access log: %w", err)
	}
	return p, nil
}

func (s *Service) loadPack(ctx context.Context, kind string, packID int64) (Pack, error) {
	p := Pack{Kind: kind, ID: packID, Items: []PackItem{}, Reports: []PackReport{}}
	var ownerID *int64
	var err error
	if kind == "sticker_pack" {
		var builtin *string
		err = s.app.QueryRow(ctx, `SELECT title, slug, owner_id, builtin_key, hidden_at, hidden_by, deleted_at, created_at
FROM sticker_packs WHERE id = $1`, packID).Scan(&p.Title, &p.Slug, &ownerID, &builtin, &p.HiddenAt, &p.HiddenBy, &p.DeletedAt, &p.CreatedAt)
		p.Builtin = builtin != nil
	} else {
		var chatID *int64
		err = s.app.QueryRow(ctx, `SELECT title, slug, owner_id, chat_id, hidden_at, hidden_by, deleted_at, created_at
FROM emoji_packs WHERE id = $1`, packID).Scan(&p.Title, &p.Slug, &ownerID, &chatID, &p.HiddenAt, &p.HiddenBy, &p.DeletedAt, &p.CreatedAt)
		p.GroupPack = chatID != nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return Pack{}, ErrNotFound
	}
	if err != nil {
		return Pack{}, err
	}
	if ownerID != nil {
		var owner review.UserSummary
		err := s.app.QueryRow(ctx, `SELECT id, username, display_name, created_at FROM users WHERE id = $1`, *ownerID).
			Scan(&owner.ID, &owner.Username, &owner.DisplayName, &owner.CreatedAt)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return Pack{}, err
		}
		if err == nil {
			p.Owner = &owner
		}
	}
	itemsSQL := `SELECT id, attachment_id::text, emoji, is_animated FROM stickers WHERE pack_id = $1 AND deleted_at IS NULL ORDER BY position, id`
	reportsSQL := `SELECT r.reporter_id, u.username, r.reason, r.created_at FROM sticker_pack_reports r JOIN users u ON u.id = r.reporter_id
WHERE r.pack_id = $1 ORDER BY r.created_at DESC`
	if kind == "emoji_pack" {
		itemsSQL = `SELECT id, attachment_id::text, shortcode, is_animated FROM custom_emoji WHERE pack_id = $1 AND deleted_at IS NULL ORDER BY position, id`
		reportsSQL = `SELECT r.reporter_id, u.username, r.reason, r.created_at FROM emoji_pack_reports r JOIN users u ON u.id = r.reporter_id
WHERE r.pack_id = $1 ORDER BY r.created_at DESC`
	}
	rows, err := s.app.Query(ctx, itemsSQL, packID)
	if err != nil {
		return Pack{}, err
	}
	if p.Items, err = pgx.CollectRows(rows, pgx.RowToStructByPos[PackItem]); err != nil {
		return Pack{}, err
	}
	rows, err = s.app.Query(ctx, reportsSQL, packID)
	if err != nil {
		return Pack{}, err
	}
	if p.Reports, err = pgx.CollectRows(rows, pgx.RowToStructByPos[PackReport]); err != nil {
		return Pack{}, err
	}
	return p, nil
}

type ReportedPack struct {
	Kind     string     `json:"kind"`
	ID       int64      `json:"id,string"`
	Title    string     `json:"title"`
	Reports  int        `json:"reports"`
	HiddenAt *time.Time `json:"hidden_at"`
	HiddenBy *string    `json:"hidden_by"`
	LastAt   *time.Time `json:"last_report_at"`
}

func (s *Service) ReportedPacks(ctx context.Context, st staff.Staff) ([]ReportedPack, error) {
	if s.app == nil {
		return nil, ErrUnavailable
	}
	rows, err := s.app.Query(ctx, `SELECT 'sticker_pack', p.id, p.title, count(r.reporter_id)::int, p.hidden_at, p.hidden_by, max(r.created_at)
FROM sticker_packs p LEFT JOIN sticker_pack_reports r ON r.pack_id = p.id
WHERE p.deleted_at IS NULL AND (p.hidden_at IS NOT NULL OR r.pack_id IS NOT NULL)
GROUP BY p.id
UNION ALL
SELECT 'emoji_pack', p.id, p.title, count(r.reporter_id)::int, p.hidden_at, p.hidden_by, max(r.created_at)
FROM emoji_packs p LEFT JOIN emoji_pack_reports r ON r.pack_id = p.id
WHERE p.deleted_at IS NULL AND (p.hidden_at IS NOT NULL OR r.pack_id IS NOT NULL)
GROUP BY p.id
ORDER BY 4 DESC, 7 DESC NULLS LAST
LIMIT 200`)
	if err != nil {
		return nil, err
	}
	list, err := pgx.CollectRows(rows, pgx.RowToStructByPos[ReportedPack])
	if err != nil {
		return nil, err
	}
	if err := audit.Write(ctx, s.mod, audit.Entry{StaffID: &st.ID, Action: "packs.viewed", Details: map[string]any{"count": len(list)}}); err != nil {
		return nil, fmt.Errorf("data access log: %w", err)
	}
	return list, nil
}

func (s *Service) PackImage(ctx context.Context, kind string, packID int64, attachmentID string) (*http.Response, error) {
	if !slices.Contains(PackKinds, kind) {
		return nil, ErrNotFound
	}
	if !isUUID(attachmentID) {
		return nil, ErrNotFound
	}
	if s.app == nil {
		return nil, ErrUnavailable
	}
	table := "stickers"
	if kind == "emoji_pack" {
		table = "custom_emoji"
	}
	var ok bool
	if err := s.app.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM `+table+` WHERE pack_id = $1 AND attachment_id = $2::uuid)`, packID, attachmentID).Scan(&ok); err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrNotFound
	}
	resp, err := s.messenger.PackImage(ctx, attachmentID)
	if err != nil {
		return nil, s.messengerError(err)
	}
	return resp, nil
}

func CopyImage(w http.ResponseWriter, resp *http.Response) {
	h := w.Header()
	ctype := resp.Header.Get("Content-Type")
	switch ctype {
	case "image/webp", "image/png", "image/gif", "image/jpeg":
	default:
		ctype = "application/octet-stream"
	}
	h.Set("Content-Type", ctype)
	h.Set("Cache-Control", "private, max-age=300")
	h.Set("Content-Security-Policy", "default-src 'none'; sandbox")
	h.Set("X-Content-Type-Options", "nosniff")
	if n := resp.Header.Get("Content-Length"); n != "" {
		h.Set("Content-Length", n)
	}
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, resp.Body)
}

func (s *Service) Reviews(ctx context.Context) ([]review.Review, error) {
	rows, err := s.mod.Query(ctx, `SELECT target_id::bigint, max(created_at) FROM audit_log
WHERE action IN ('user.status_set', 'user.reports_dismissed') AND target_type = 'user'
GROUP BY target_id`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[review.Review])
}

func isUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return false
			}
		default:
			if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
				return false
			}
		}
	}
	return true
}
