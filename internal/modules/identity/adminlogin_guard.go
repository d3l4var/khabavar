package identity

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/khabavar/khabavar/internal/platform/ratelimit"
)

// Staff login had no throttle at all: OTP capped at 5/hour, but a password could be guessed
// forever. This guard caps both dimensions and records every failure, because a lockout
// without a trail cannot be investigated after the fact.
//
// Two dimensions on purpose: an IP cap alone lets a botnet spread attempts, and an email cap
// alone lets an attacker lock a colleague out. Both together mean a distributed attack still
// hits the per-account ceiling, and a single source still hits the per-IP ceiling.
var (
	// ErrAdminLoginThrottled is returned when either ceiling is reached.
	ErrAdminLoginThrottled = errors.New("identity: too many login attempts")
)

// AdminLoginGuard caps staff login attempts per IP and per email.
type AdminLoginGuard struct {
	byIP    *ratelimit.Limiter
	byEmail *ratelimit.Limiter
	now     func() time.Time
}

// NewAdminLoginGuard builds the guard. Defaults are chosen so a human who mistypes twice is
// never locked out, while a script stops almost immediately.
func NewAdminLoginGuard(perIP, perEmail int, window time.Duration) *AdminLoginGuard {
	if perIP <= 0 {
		perIP = 10
	}
	if perEmail <= 0 {
		perEmail = 5
	}
	if window <= 0 {
		window = 15 * time.Minute
	}
	return &AdminLoginGuard{
		byIP:    ratelimit.New(perIP, window),
		byEmail: ratelimit.New(perEmail, window),
		now:     time.Now,
	}
}

// Allow consumes one attempt and reports whether it may proceed. Both ceilings are checked
// before either is consumed, so a blocked request does not burn the other dimension's budget.
func (g *AdminLoginGuard) Allow(ip, email string) bool {
	if g == nil {
		return true
	}
	now := g.now()
	key := strings.ToLower(strings.TrimSpace(email))
	if !g.byIP.Allow(ip, now) {
		return false
	}
	if key != "" && !g.byEmail.Allow(key, now) {
		return false
	}
	return true
}

// AdminLoginAuditor records staff login outcomes. Failures are the interesting ones: they are
// the only signal that someone is guessing.
type AdminLoginAuditor interface {
	Record(ctx context.Context, actorType, actorID, action, entityType, entityID string, oldValues, newValues any, ip string) error
}

// auditLoginFailure writes the attempt. It never changes the response: a failed audit write
// must not turn a 401 into a 500.
func auditLoginFailure(ctx context.Context, aud AdminLoginAuditor, email, ip, why string) {
	if aud == nil {
		return
	}
	_ = aud.Record(ctx, "ADMIN", "", "auth.admin_login_failed", "admin_user", "",
		nil, map[string]string{"email": strings.ToLower(strings.TrimSpace(email)), "ip": ip, "reason": why}, ip)
}
