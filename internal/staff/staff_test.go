package staff

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"

	"github.com/myronsi/messenger-moderation/internal/testdb"
)

var testKey = []byte("0123456789abcdef0123456789abcdef")

func newService(t *testing.T) (*Service, context.Context) {
	t.Helper()
	pool, _ := testdb.New(t)
	svc, err := NewService(pool, testKey, 12*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return svc, context.Background()
}

func createAdmin(t *testing.T, svc *Service, ctx context.Context) Created {
	t.Helper()
	c, err := svc.Create(ctx, NewStaff{Username: "Operator", Password: "a long enough password", Role: RoleAdmin})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func code(t *testing.T, secret string, at time.Time) string {
	t.Helper()
	c, err := totp.GenerateCode(secret, at)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func auditActions(t *testing.T, svc *Service, ctx context.Context) []string {
	t.Helper()
	rows, err := svc.pool.Query(ctx, `SELECT action FROM audit_log ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var actions []string
	for rows.Next() {
		var a string
		if err := rows.Scan(&a); err != nil {
			t.Fatal(err)
		}
		actions = append(actions, a)
	}
	return actions
}

func TestCreateValidatesAndRequiresAgreementForModerators(t *testing.T) {
	svc, ctx := newService(t)
	signed := time.Now()

	cases := map[string]NewStaff{
		"short username": {Username: "ab", Password: "a long enough password", Role: RoleAdmin},
		"short password": {Username: "valid_name", Password: "short", Role: RoleAdmin},
		"unknown role":   {Username: "valid_name", Password: "a long enough password", Role: "owner"},
	}
	for name, n := range cases {
		var v *ValidationError
		if _, err := svc.Create(ctx, n); !errors.As(err, &v) {
			t.Errorf("%s: err = %v, want a validation error", name, err)
		}
	}
	if _, err := svc.Create(ctx, NewStaff{Username: "helper", Password: "a long enough password", Role: RoleModerator}); !errors.Is(err, ErrNeedsAgreement) {
		t.Fatalf("moderator without agreement: err = %v", err)
	}
	mod, err := svc.Create(ctx, NewStaff{Username: "helper", Password: "a long enough password", Role: RoleModerator, ConfidentialityOn: &signed})
	if err != nil {
		t.Fatal(err)
	}
	if mod.Staff.Role != RoleModerator || mod.TOTPSecret == "" || !strings.HasPrefix(mod.TOTPURI, "otpauth://totp/") {
		t.Fatalf("created = %+v", mod)
	}
	if _, err := svc.Create(ctx, NewStaff{Username: "HELPER", Password: "a long enough password", Role: RoleAdmin}); !errors.Is(err, ErrUsernameTaken) {
		t.Fatalf("duplicate: err = %v", err)
	}
}

func TestLoginNeedsPasswordAndCode(t *testing.T) {
	svc, ctx := newService(t)
	admin := createAdmin(t, svc, ctx)
	now := time.Now()

	if _, err := svc.Login(ctx, "operator", "wrong password!", code(t, admin.TOTPSecret, now), "ip", "ua"); !errors.Is(err, ErrInvalidLogin) {
		t.Fatalf("wrong password: err = %v", err)
	}
	if _, err := svc.Login(ctx, "operator", "a long enough password", "000000", "ip", "ua"); !errors.Is(err, ErrInvalidLogin) {
		t.Fatalf("wrong code: err = %v", err)
	}
	if _, err := svc.Login(ctx, "nobody", "a long enough password", "123456", "ip", "ua"); !errors.Is(err, ErrInvalidLogin) {
		t.Fatalf("unknown user: err = %v", err)
	}

	session, err := svc.Login(ctx, " Operator ", "a long enough password", code(t, admin.TOTPSecret, now), "ip", "ua")
	if err != nil {
		t.Fatal(err)
	}
	if session.Token == "" || session.Staff.Role != RoleAdmin || session.ExpiresAt.Sub(now) < 11*time.Hour {
		t.Fatalf("session = %+v", session)
	}
	got, err := svc.Authenticate(ctx, session.Token)
	if err != nil || got.ID != admin.Staff.ID {
		t.Fatalf("authenticate = %+v, %v", got, err)
	}

	if err := svc.Logout(ctx, session.Token); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Authenticate(ctx, session.Token); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("after logout: err = %v", err)
	}

	want := []string{"staff.created", "staff.login_failed", "staff.login_failed", "staff.login_failed", "staff.login", "staff.logout"}
	if got := auditActions(t, svc, ctx); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("audit = %v, want %v", got, want)
	}
}

func TestSessionsExpire(t *testing.T) {
	svc, ctx := newService(t)
	admin := createAdmin(t, svc, ctx)
	start := time.Now()
	svc.now = func() time.Time { return start }

	session, err := svc.Login(ctx, "operator", "a long enough password", code(t, admin.TOTPSecret, start), "ip", "ua")
	if err != nil {
		t.Fatal(err)
	}
	svc.now = func() time.Time { return start.Add(12*time.Hour + time.Second) }
	if _, err := svc.Authenticate(ctx, session.Token); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("expired session: err = %v", err)
	}
	if n, err := svc.PurgeExpiredSessions(ctx); err != nil || n != 1 {
		t.Fatalf("purge = %d, %v", n, err)
	}
}

func TestDisabledStaffCannotSignIn(t *testing.T) {
	svc, ctx := newService(t)
	admin := createAdmin(t, svc, ctx)
	session, err := svc.Login(ctx, "operator", "a long enough password", code(t, admin.TOTPSecret, time.Now()), "ip", "ua")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.pool.Exec(ctx, `UPDATE staff_accounts SET disabled_at = now() WHERE id = $1`, admin.Staff.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Authenticate(ctx, session.Token); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("disabled session: err = %v", err)
	}
	if _, err := svc.Login(ctx, "operator", "a long enough password", code(t, admin.TOTPSecret, time.Now()), "ip2", "ua"); !errors.Is(err, ErrInvalidLogin) {
		t.Fatalf("disabled login: err = %v", err)
	}
}

func TestTooManyFailedAttempts(t *testing.T) {
	svc, ctx := newService(t)
	admin := createAdmin(t, svc, ctx)
	for i := 0; i < maxFailures; i++ {
		_, _ = svc.Login(ctx, "operator", "wrong password!", "000000", "1.2.3.4", "ua")
	}
	if _, err := svc.Login(ctx, "operator", "a long enough password", code(t, admin.TOTPSecret, time.Now()), "1.2.3.4", "ua"); !errors.Is(err, ErrTooManyAttempts) {
		t.Fatalf("err = %v, want too many attempts", err)
	}
	if _, err := svc.Login(ctx, "operator", "a long enough password", code(t, admin.TOTPSecret, time.Now()), "5.6.7.8", "ua"); err != nil {
		t.Fatalf("other client must still sign in: %v", err)
	}
}

func TestAuditLogIsAppendOnly(t *testing.T) {
	svc, ctx := newService(t)
	createAdmin(t, svc, ctx)
	if _, err := svc.pool.Exec(ctx, `UPDATE audit_log SET action = 'changed'`); err == nil {
		t.Fatal("update of the audit log must fail")
	}
	if _, err := svc.pool.Exec(ctx, `DELETE FROM audit_log`); err == nil {
		t.Fatal("delete of recent audit rows must fail")
	}
}
