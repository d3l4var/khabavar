// Authentication and audited authorization middleware (IAM §۱, §۷).
//
// Authenticate turns a Bearer access token into an admin identity in the request
// context; Require/RequireAudited then check the permission code for the route. Every
// admin route must pass through both — there is no panel path without a permission
// check (IAM §۱), and a denial is recorded in the audit log (IAM §۷).
package authorization

import (
	"context"
	"net/http"
	"strings"
)

// SubjectResolver validates a bearer access token and returns the subject kind
// ("admin" or "customer") plus its id. The identity module's DBSubjectResolver
// satisfies this shape through a small adapter in main.
type SubjectResolver func(ctx context.Context, bearerToken string) (kind, id string, ok bool)

// ActorTypeAdmin is the audit_logs.actor_type used for staff actions.
const ActorTypeAdmin = "ADMIN"

// Auditor records an audit entry. *audit.Recorder satisfies this interface; declaring
// it here keeps the authorization package free of a hard dependency on audit.
type Auditor interface {
	Record(ctx context.Context, actorType, actorID, action, entityType, entityID string, oldValues, newValues any, ip string) error
}

// Authenticate resolves the Bearer token and stores the admin user id in ctx.
// Customer tokens are rejected: a shop session must never open the panel (IAM §۶).
// Missing, malformed, expired or revoked tokens all yield 401.
func Authenticate(resolve SubjectResolver, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bearer := BearerFromHeader(r)
		if bearer == "" {
			writeAuthErr(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		kind, id, ok := resolve(r.Context(), bearer)
		if !ok || kind != "admin" || id == "" {
			writeAuthErr(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next.ServeHTTP(w, r.WithContext(WithAdminUserID(r.Context(), id)))
	})
}

// RequireAudited is Require plus an audit record on denial (IAM §۷: 403 + audit).
// A nil auditor degrades to Require so tests and read-only deployments stay simple.
func RequireAudited(chk PermissionChecker, aud Auditor, permission string, next http.Handler) http.Handler {
	if aud == nil {
		return Require(chk, permission, next)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uid := AdminUserID(r.Context())
		if uid == "" {
			writeAuthErr(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		ok, err := chk.HasPermission(r.Context(), uid, permission)
		if err != nil || !ok {
			// Denials are audited; the audit write must not mask the 403.
			// entity_id is a UUID column, so the permission code rides in new_values.
			_ = aud.Record(r.Context(), ActorTypeAdmin, uid, "authorization.denied", "permission", "",
				nil, map[string]string{
					"permission": permission,
					"method":     r.Method,
					"path":       r.URL.Path,
				}, clientIP(r))
			writeAuthErr(w, http.StatusForbidden, "forbidden")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// BearerFromHeader extracts the token from an "Authorization: Bearer <token>" header.
func BearerFromHeader(r *http.Request) string {
	v := r.Header.Get("Authorization")
	if v == "" {
		return ""
	}
	if len(v) > 7 && strings.EqualFold(v[:7], "bearer ") {
		return strings.TrimSpace(v[7:])
	}
	return ""
}

// clientIP prefers the proxy-supplied X-Forwarded-For head (nginx sets it) and falls
// back to the socket address.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.IndexByte(xff, ','); i > 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	host := r.RemoteAddr
	if i := strings.LastIndexByte(host, ':'); i > 0 {
		return host[:i]
	}
	return host
}

func writeAuthErr(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("WWW-Authenticate", `Bearer realm="khabavar-admin"`)
	w.WriteHeader(status)
	_, _ = w.Write([]byte(msg + "\n"))
}
