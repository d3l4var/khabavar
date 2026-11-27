package authorization

import (
	"context"
	"net/http"
)

// customerKey is the context key for the authenticated customer id.
type customerKey struct{}

// WithCustomerID stores the authenticated customer id in the request context.
func WithCustomerID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, customerKey{}, id)
}

// CustomerID returns the authenticated customer id, or "" when anonymous.
func CustomerID(ctx context.Context) string {
	id, _ := ctx.Value(customerKey{}).(string)
	return id
}

// AuthenticateCustomer requires a bearer token that resolves to a *customer* subject.
// An admin token is rejected here on purpose: the customer surface must not be a back
// door into staff identity, and vice versa.
func AuthenticateCustomer(resolve SubjectResolver, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bearer := BearerFromHeader(r)
		if bearer == "" {
			writeAuthErr(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		kind, id, ok := resolve(r.Context(), bearer)
		if !ok || kind != "customer" || id == "" {
			writeAuthErr(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next.ServeHTTP(w, r.WithContext(WithCustomerID(r.Context(), id)))
	})
}

// OptionalCustomer attaches the customer identity when a valid customer bearer is present and
// lets the request through anonymously otherwise. The cart and checkout routes are public
// because a guest must be able to use them, but a signed-in customer must not be treated as a
// guest — without this the handler saw an empty CustomerID and demanded a guest token.
func OptionalCustomer(resolve SubjectResolver, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bearer := BearerFromHeader(r)
		if bearer == "" {
			next.ServeHTTP(w, r)
			return
		}
		kind, id, ok := resolve(r.Context(), bearer)
		if !ok || kind != "customer" || id == "" {
			// A staff token or a bad token is not an error here: the route stays anonymous.
			next.ServeHTTP(w, r)
			return
		}
		next.ServeHTTP(w, r.WithContext(WithCustomerID(r.Context(), id)))
	})
}
