// Admin route authorization table (IAM §۱, §۴).
//
// Every /api/v1/admin/* route is mounted through apiMux.admin, which looks its
// permission code up in adminPermissions and wraps the handler with
// authorization.Authenticate + authorization.RequireAudited. Mounting an admin route
// that has no entry here panics at startup, so a panel endpoint can never ship without
// a permission check — the invariant IAM §۱ states and TS §۴ tests.
package main

import (
	"fmt"
	"net/http"

	"github.com/khabavar/khabavar/internal/modules/authorization"
)

// adminPermissions maps each mounted admin route pattern to the seed permission code
// it requires. Codes come from 001_seed_baseline.sql (24 permissions, IAM §۳); the
// server never checks role names (IAM §۱).
var adminPermissions = map[string]string{
	// ── commerce: refunds are FINANCE-exclusive, reconciliation is a finance read ──
	"POST /api/v1/admin/orders/{id}/refunds":  "order.refund.approve",
	"GET /api/v1/admin/orders/{id}/refunds":   "order.view",
	"GET /api/v1/admin/orders/{id}/reconcile": "reports.financial.view",

	// ── suppliers (SUPPLIER_OPERATOR) ──
	"POST /api/v1/admin/suppliers":                           "supplier.manage",
	"PATCH /api/v1/admin/suppliers/{id}/status":              "supplier.manage",
	"GET /api/v1/admin/supplier-variants/{id}/price-history": "supplier.manage",
	"POST /api/v1/admin/suppliers/{id}/evaluations":          "supplier.evaluate",
	"GET /api/v1/admin/suppliers/{id}/evaluations":           "supplier.manage",

	// ── inventory (INVENTORY_MANAGER; adjustment is the tighter code) ──
	"POST /api/v1/admin/warehouses":                               "inventory.manage",
	"POST /api/v1/admin/warehouses/{wid}/variants/{vid}/receive":  "inventory.manage",
	"POST /api/v1/admin/warehouses/{wid}/variants/{vid}/adjust":   "inventory.adjust",
	"POST /api/v1/admin/inventory/reserve":                        "inventory.manage",
	"DELETE /api/v1/admin/inventory/reservations/{id}":            "inventory.manage",
	"POST /api/v1/admin/inventory/reservations/{id}/convert":      "inventory.manage",
	"GET /api/v1/admin/warehouses/{wid}/variants/{vid}/movements": "inventory.manage",

	// ── fulfillment (ORDER_OPERATOR) ──
	"POST /api/v1/admin/fulfillment-orders":                 "order.manage",
	"POST /api/v1/admin/fulfillment-orders/{id}/confirm":    "order.manage",
	"POST /api/v1/admin/fulfillment-orders/{id}/processing": "order.manage",
	"POST /api/v1/admin/fulfillment-orders/{id}/ready":      "order.manage",
	"POST /api/v1/admin/fulfillment-orders/{id}/shipped":    "order.manage",
	"POST /api/v1/admin/fulfillment-orders/{id}/delivered":  "order.manage",
	"POST /api/v1/admin/fulfillment-orders/{id}/cancel":     "order.manage",
	"POST /api/v1/admin/fulfillment-orders/{id}/reject":     "order.manage",
	"POST /api/v1/admin/fulfillment-orders/{id}/shipments":  "order.manage",
	"POST /api/v1/admin/shipments/{id}/transition":          "order.manage",

	// ── media (catalog owners) ──
	"POST /api/v1/admin/media":        "catalog.product.edit",
	"DELETE /api/v1/admin/media/{id}": "catalog.product.edit",

	// ── content / magazine editorial workflow ──
	"POST /api/v1/admin/topics":                  "content.article.edit",
	"POST /api/v1/admin/articles":                "content.article.edit",
	"POST /api/v1/admin/articles/{id}/submit":    "content.article.edit",
	"POST /api/v1/admin/articles/{id}/approve":   "content.article.review",
	"POST /api/v1/admin/articles/{id}/schedule":  "content.article.publish",
	"POST /api/v1/admin/articles/{id}/publish":   "content.article.publish",
	"POST /api/v1/admin/articles/{id}/unpublish": "content.article.publish",
	"POST /api/v1/admin/articles/{id}/archive":   "content.article.publish",
	"POST /api/v1/admin/articles/{id}/reject":    "content.article.review",
	"POST /api/v1/admin/articles/{id}/sources":   "content.article.edit",
	"POST /api/v1/admin/articles/{id}/topics":    "content.article.edit",
	"POST /api/v1/admin/articles/{id}/products":  "content.relation.manage",
}

// seedPermissionCodes is the 24-permission catalogue from 001_seed_baseline.sql
// (IAM §۳). The RBAC test gate asserts every adminPermissions value is one of these,
// so a typo cannot silently lock a role out of an endpoint.
var seedPermissionCodes = []string{
	"catalog.product.view", "catalog.product.edit", "catalog.product.publish",
	"catalog.attribute.manage",
	"supplier.manage", "supplier.evaluate",
	"inventory.manage", "inventory.adjust",
	"pricing.cost.manage", "pricing.price.approve",
	"order.view", "order.manage", "order.cancel.approve", "order.refund.approve",
	"content.article.edit", "content.article.review", "content.article.publish",
	"content.relation.manage",
	"identity.user.manage", "identity.customer.view",
	"reports.view", "reports.financial.view",
	"audit.read", "settings.manage",
}

// apiMux mounts routes on a ServeMux, enforcing the admin authorization invariant.
type apiMux struct {
	mux     *http.ServeMux
	guard   func(permission string, next http.Handler) http.Handler
	mounted map[string]string
}

// newAPIMux builds the mux wrapper. guard must authenticate the caller and check the
// permission code; runAPI passes authorization.Authenticate ∘ RequireAudited.
func newAPIMux(mux *http.ServeMux, guard func(string, http.Handler) http.Handler) *apiMux {
	return &apiMux{mux: mux, guard: guard, mounted: map[string]string{}}
}

// public mounts a route that needs no staff permission (storefront reads, health).
func (a *apiMux) public(pattern string, h http.Handler) {
	a.mux.Handle(pattern, h)
	a.mounted[pattern] = ""
}

// admin mounts a staff route behind authentication + the permission mapped to pattern.
// Panics when the pattern has no permission entry: an unguarded panel route is a
// security defect, so it must fail loudly at startup, never silently ship.
func (a *apiMux) admin(pattern string, h http.Handler) {
	perm, ok := adminPermissions[pattern]
	if !ok || perm == "" {
		panic(fmt.Sprintf("apiMux.admin: no permission mapped for %q — add it to adminPermissions (IAM §۱)", pattern))
	}
	a.mux.Handle(pattern, a.guard(perm, h))
	a.mounted[pattern] = perm
}

// newAdminGuard composes authentication with the audited permission check.
func newAdminGuard(resolve authorization.SubjectResolver, chk authorization.PermissionChecker, aud authorization.Auditor) func(string, http.Handler) http.Handler {
	return func(permission string, next http.Handler) http.Handler {
		return authorization.Authenticate(resolve,
			authorization.RequireAudited(chk, aud, permission, next))
	}
}
