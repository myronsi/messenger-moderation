package staff

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"

	"github.com/myronsi/messenger-moderation/internal/audit"
)

type Role string

const (
	RoleAdmin     Role = "admin"
	RoleModerator Role = "moderator"
)

const (
	minPasswordLen = 12
	maxPasswordLen = 128
	totpIssuer     = "Messenger moderation"
	maxFailures    = 10
	failureWindow  = 15 * time.Minute
)

var (
	ErrInvalidLogin     = errors.New("invalid username, password or code")
	ErrTooManyAttempts  = errors.New("too many failed sign-in attempts, try again later")
	ErrUnauthenticated  = errors.New("not signed in")
	ErrUsernameTaken    = errors.New("username already exists")
	ErrNeedsAgreement   = errors.New("moderators need a signed confidentiality promise")
	usernamePattern     = regexp.MustCompile(`^[a-z0-9_]{3,32}$`)
	errInvalidRoleValue = errors.New("role must be admin or moderator")
)

type ValidationError struct{ Field, Message string }

func (e *ValidationError) Error() string { return e.Field + ": " + e.Message }

type Staff struct {
	ID                      int64      `json:"id"`
	Username                string     `json:"username"`
	Role                    Role       `json:"role"`
	ConfidentialitySignedAt *time.Time `json:"confidentiality_signed_at,omitempty"`
}

func (s Staff) IsAdmin() bool { return s.Role == RoleAdmin }

type NewStaff struct {
	Username          string
	Password          string
	Role              Role
	ConfidentialityOn *time.Time
	CreatedBy         *int64
}

type Created struct {
	Staff      Staff
	TOTPSecret string
	TOTPURI    string
}

type Session struct {
	Staff     Staff
	Token     string
	ExpiresAt time.Time
}

type Service struct {
	pool    *pgxpool.Pool
	sealer  *sealer
	ttl     time.Duration
	now     func() time.Time
	limiter *limiter
}

func NewService(pool *pgxpool.Pool, key []byte, ttl time.Duration) (*Service, error) {
	s, err := newSealer(key)
	if err != nil {
		return nil, fmt.Errorf("staff: %w", err)
	}
	now := time.Now
	return &Service{pool: pool, sealer: s, ttl: ttl, now: now, limiter: newLimiter(maxFailures, failureWindow, now)}, nil
}

func (s *Service) Create(ctx context.Context, n NewStaff) (Created, error) {
	username := strings.ToLower(strings.TrimSpace(n.Username))
	if !usernamePattern.MatchString(username) {
		return Created{}, &ValidationError{"username", "3 to 32 lowercase letters, digits or _"}
	}
	if l := utf8.RuneCountInString(n.Password); l < minPasswordLen || l > maxPasswordLen {
		return Created{}, &ValidationError{"password", fmt.Sprintf("must be %d to %d characters", minPasswordLen, maxPasswordLen)}
	}
	if n.Role != RoleAdmin && n.Role != RoleModerator {
		return Created{}, &ValidationError{"role", errInvalidRoleValue.Error()}
	}
	if n.Role == RoleModerator && n.ConfidentialityOn == nil {
		return Created{}, ErrNeedsAgreement
	}
	hash, err := hashPassword(n.Password)
	if err != nil {
		return Created{}, err
	}
	key, err := totp.Generate(totp.GenerateOpts{Issuer: totpIssuer, AccountName: username})
	if err != nil {
		return Created{}, err
	}
	sealed, err := s.sealer.seal([]byte(key.Secret()), []byte("totp:"+username))
	if err != nil {
		return Created{}, err
	}

	var created Created
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `INSERT INTO staff_accounts (username, password_hash, totp_secret, role, confidentiality_signed_at, created_by)
			VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`, username, hash, sealed, string(n.Role), n.ConfidentialityOn, n.CreatedBy)
		var id int64
		if err := row.Scan(&id); err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				return ErrUsernameTaken
			}
			return err
		}
		created = Created{
			Staff:      Staff{ID: id, Username: username, Role: n.Role, ConfidentialitySignedAt: n.ConfidentialityOn},
			TOTPSecret: key.Secret(),
			TOTPURI:    key.URL(),
		}
		return audit.Write(ctx, tx, audit.Entry{
			StaffID: n.CreatedBy, Action: "staff.created", TargetType: "staff", TargetID: strconv.FormatInt(id, 10),
			Details: map[string]any{"username": username, "role": n.Role},
		})
	})
	return created, err
}

func (s *Service) Login(ctx context.Context, username, password, code, client, userAgent string) (Session, error) {
	username = strings.ToLower(strings.TrimSpace(username))
	limitKey := username + "|" + client
	if s.limiter.blocked(limitKey) {
		return Session{}, ErrTooManyAttempts
	}

	var (
		st       Staff
		hash     string
		sealed   []byte
		disabled *time.Time
	)
	err := s.pool.QueryRow(ctx, `SELECT id, username, role, confidentiality_signed_at, password_hash, totp_secret, disabled_at
		FROM staff_accounts WHERE username = $1`, username).
		Scan(&st.ID, &st.Username, &st.Role, &st.ConfidentialitySignedAt, &hash, &sealed, &disabled)
	found := err == nil
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return Session{}, err
	}
	if !found {
		hash = dummyHash
	}

	ok := verifyPassword(hash, password) && found && disabled == nil
	if ok {
		secret, err := s.sealer.open(sealed, []byte("totp:"+username))
		if err != nil {
			return Session{}, fmt.Errorf("staff: open totp secret: %w", err)
		}
		ok, _ = totp.ValidateCustom(strings.TrimSpace(code), string(secret), s.now(), totp.ValidateOpts{
			Period: 30, Skew: 1, Digits: otp.DigitsSix, Algorithm: otp.AlgorithmSHA1,
		})
	}
	if !ok {
		s.limiter.fail(limitKey)
		var staffID *int64
		if found {
			staffID = &st.ID
		}
		_ = audit.Write(ctx, s.pool, audit.Entry{StaffID: staffID, Action: "staff.login_failed", Details: map[string]any{"username": username}})
		return Session{}, ErrInvalidLogin
	}
	s.limiter.reset(limitKey)

	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return Session{}, err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	expires := s.now().Add(s.ttl)
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO staff_sessions (token_hash, staff_id, expires_at, user_agent) VALUES ($1, $2, $3, $4)`,
			tokenHash(token), st.ID, expires, truncate(userAgent, 300)); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{StaffID: &st.ID, Action: "staff.login"})
	})
	if err != nil {
		return Session{}, err
	}
	return Session{Staff: st, Token: token, ExpiresAt: expires}, nil
}

func (s *Service) Authenticate(ctx context.Context, token string) (Staff, error) {
	if token == "" {
		return Staff{}, ErrUnauthenticated
	}
	var st Staff
	err := s.pool.QueryRow(ctx, `SELECT a.id, a.username, a.role, a.confidentiality_signed_at
		FROM staff_sessions s JOIN staff_accounts a ON a.id = s.staff_id
		WHERE s.token_hash = $1 AND s.expires_at > $2 AND a.disabled_at IS NULL`, tokenHash(token), s.now()).
		Scan(&st.ID, &st.Username, &st.Role, &st.ConfidentialitySignedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Staff{}, ErrUnauthenticated
	}
	return st, err
}

func (s *Service) Logout(ctx context.Context, token string) error {
	var staffID int64
	err := s.pool.QueryRow(ctx, `DELETE FROM staff_sessions WHERE token_hash = $1 RETURNING staff_id`, tokenHash(token)).Scan(&staffID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	return audit.Write(ctx, s.pool, audit.Entry{StaffID: &staffID, Action: "staff.logout"})
}

func (s *Service) PurgeExpiredSessions(ctx context.Context) (int64, error) {
	tag, err := s.pool.Exec(ctx, `DELETE FROM staff_sessions WHERE expires_at <= $1`, s.now())
	return tag.RowsAffected(), err
}

func tokenHash(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}
