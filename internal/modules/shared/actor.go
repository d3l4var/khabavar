// actor.go carries the authenticated staff/customer identity across module
// boundaries without importing the authorization package (SAD §۴.۲: every module may
// depend on `shared`, and only on its listed modules otherwise).
//
// The admin guard in cmd/khabavar authenticates the Bearer token and stores the admin
// id here; modules that must record who performed a sensitive action (refunds, price
// approvals, stock adjustments — IAM §۷) read it back with ActorID.
package shared

import "context"

type actorKey struct{}

// WithActorID stores the acting principal's id in ctx.
func WithActorID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, actorKey{}, id)
}

// ActorID returns the acting principal's id, or "" when the call is anonymous
// (storefront, background job).
func ActorID(ctx context.Context) string {
	v, _ := ctx.Value(actorKey{}).(string)
	return v
}
