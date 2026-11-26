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
	"github.com/khabavar/khabavar/internal/modules/catalog"
	"github.com/khabavar/khabavar/internal/modules/commerce"
	"github.com/khabavar/khabavar/internal/modules/content"
	"github.com/khabavar/khabavar/internal/modules/ecosystem"
	"github.com/khabavar/khabavar/internal/modules/fulfillment"
	"github.com/khabavar/khabavar/internal/modules/inventory"
	"github.com/khabavar/khabavar/internal/modules/media"
	"github.com/khabavar/khabavar/internal/modules/pricing"
	"github.com/khabavar/khabavar/internal/modules/shared"
	"github.com/khabavar/khabavar/internal/modules/suppliers"
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

// newAdminGuard composes authentication with the audited permission check and copies
// the authenticated admin id into the shared actor context, so modules can record who
// performed a sensitive action (IAM §۷).
func newAdminGuard(resolve authorization.SubjectResolver, chk authorization.PermissionChecker, aud authorization.Auditor) func(string, http.Handler) http.Handler {
	return func(permission string, next http.Handler) http.Handler {
		return authorization.Authenticate(resolve,
			withActor(authorization.RequireAudited(chk, aud, permission, next)))
	}
}

// withActor stores the authenticated admin id in the shared actor context.
func withActor(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if id := authorization.AdminUserID(r.Context()); id != "" {
			r = r.WithContext(shared.WithActorID(r.Context(), id))
		}
		next.ServeHTTP(w, r)
	})
}

// services holds every module service the api unit mounts. Keeping them in one
// struct lets mountRoutes be exercised by a test with zero values, which is how the
// route table is checked for ServeMux pattern conflicts before a deploy does it.
type services struct {
	catalog     *catalog.Service
	pricing     *pricing.Service
	commerce    *commerce.Service
	suppliers   *suppliers.Service
	inventory   *inventory.Service
	fulfillment *fulfillment.Service
	media       *media.Service
	content     *content.Service
	ecosystem   *ecosystem.Service
}

// mountRoutes registers every api-unit route. Handlers are closures over the
// services, so a zero-value services struct mounts fine (nothing is invoked) — that
// is what TestMountRoutesHasNoPatternConflicts relies on.
func mountRoutes(api *apiMux, s services) {
	// ── catalog (P5a) ──
	api.public("GET /api/v1/categories", catalog.CategoryTreeHandler(s.catalog))
	api.public("GET /api/v1/categories/{id}/products", catalog.ListProductsByCategoryHandler(s.catalog))
	api.public("GET /api/v1/products/{id}", catalog.ProductDetailHandler(s.catalog))
	api.public("GET /api/v1/products/search", catalog.SearchProductsHandler(s.catalog))

	// ── pricing (P5b) ──
	api.public("GET /api/v1/variants/{id}/price", pricing.StorefrontPriceHandler(s.pricing))

	// ── commerce (P4) ──
	api.public("POST /api/v1/orders", commerce.CreateOrderHandler(s.commerce))
	// GET /orders/{id} accepts either the internal UUID or the business order number
	// (KHA-ORD-YYYY-NNNNNN) — the contract exposes /orders/{orderNumber}. The two are
	// one route on purpose: separate /orders/by-number/{n} and /orders/{id}/fulfillment
	// patterns are ambiguous for ServeMux and panic at startup.
	api.public("GET /api/v1/orders/{id}", commerce.GetOrderHandler(s.commerce))
	api.public("POST /api/v1/orders/{id}/payment-intents", commerce.CreatePaymentIntentHandler(s.commerce))
	api.public("POST /api/v1/orders/{id}/confirm-payment", commerce.ConfirmPaymentHandler(s.commerce))
	api.public("POST /api/v1/orders/{id}/cancel", commerce.CancelOrderHandler(s.commerce))
	api.admin("POST /api/v1/admin/orders/{id}/refunds", commerce.CreateRefundHandler(s.commerce))
	api.admin("GET /api/v1/admin/orders/{id}/refunds", commerce.ListRefundsHandler(s.commerce))
	api.admin("GET /api/v1/admin/orders/{id}/reconcile", commerce.ReconcileTransactionHandler(s.commerce))

	// ── suppliers (P5c) ──
	api.admin("POST /api/v1/admin/suppliers", suppliers.CreateSupplierHandler(s.suppliers))
	api.admin("PATCH /api/v1/admin/suppliers/{id}/status", suppliers.UpdateSupplierStatusHandler(s.suppliers))
	api.public("GET /api/v1/variants/{id}/suppliers", suppliers.ListSupplierVariantsHandler(s.suppliers))
	api.admin("GET /api/v1/admin/supplier-variants/{id}/price-history", suppliers.ListPriceHistoryHandler(s.suppliers))
	api.admin("POST /api/v1/admin/suppliers/{id}/evaluations", suppliers.CreateEvaluationHandler(s.suppliers))
	api.admin("GET /api/v1/admin/suppliers/{id}/evaluations", suppliers.ListEvaluationsHandler(s.suppliers))

	// ── inventory (P5d) ──
	api.admin("POST /api/v1/admin/warehouses", inventory.CreateWarehouseHandler(s.inventory))
	api.public("GET /api/v1/warehouses/{wid}/variants/{vid}/stock", inventory.GetStockBalanceHandler(s.inventory))
	api.admin("POST /api/v1/admin/warehouses/{wid}/variants/{vid}/receive", inventory.ReceiveStockHandler(s.inventory))
	api.admin("POST /api/v1/admin/warehouses/{wid}/variants/{vid}/adjust", inventory.AdjustStockHandler(s.inventory))
	api.admin("POST /api/v1/admin/inventory/reserve", inventory.ReserveForOrderHandler(s.inventory))
	api.admin("DELETE /api/v1/admin/inventory/reservations/{id}", inventory.ReleaseReservationHandler(s.inventory))
	api.admin("POST /api/v1/admin/inventory/reservations/{id}/convert", inventory.ConvertReservationHandler(s.inventory))
	api.admin("GET /api/v1/admin/warehouses/{wid}/variants/{vid}/movements", inventory.ListMovementsHandler(s.inventory))

	// ── fulfillment (P5e) ──
	api.admin("POST /api/v1/admin/fulfillment-orders", fulfillment.CreateFulfillmentOrderHandler(s.fulfillment))
	api.admin("POST /api/v1/admin/fulfillment-orders/{id}/confirm", fulfillment.ConfirmFulfillmentHandler(s.fulfillment))
	api.admin("POST /api/v1/admin/fulfillment-orders/{id}/processing", fulfillment.StartProcessingHandler(s.fulfillment))
	api.admin("POST /api/v1/admin/fulfillment-orders/{id}/ready", fulfillment.MarkReadyHandler(s.fulfillment))
	api.admin("POST /api/v1/admin/fulfillment-orders/{id}/shipped", fulfillment.MarkShippedHandler(s.fulfillment))
	api.admin("POST /api/v1/admin/fulfillment-orders/{id}/delivered", fulfillment.MarkDeliveredHandler(s.fulfillment))
	api.admin("POST /api/v1/admin/fulfillment-orders/{id}/cancel", fulfillment.CancelFulfillmentHandler(s.fulfillment))
	api.admin("POST /api/v1/admin/fulfillment-orders/{id}/reject", fulfillment.RejectFulfillmentHandler(s.fulfillment))
	api.admin("POST /api/v1/admin/fulfillment-orders/{id}/shipments", fulfillment.CreateShipmentHandler(s.fulfillment))
	api.admin("POST /api/v1/admin/shipments/{id}/transition", fulfillment.TransitionShipmentHandler(s.fulfillment))
	api.public("GET /api/v1/shipments/{id}/events", fulfillment.ListShipmentEventsHandler(s.fulfillment))
	api.public("GET /api/v1/orders/{id}/fulfillment", fulfillment.ListFulfillmentOrdersByOrderHandler(s.fulfillment))
	api.public("GET /api/v1/fulfillment-orders/{id}/shipments", fulfillment.ListShipmentsByFulfillmentHandler(s.fulfillment))

	// ── media (P5f) ──
	api.admin("POST /api/v1/admin/media", media.UploadHandler(s.media))
	api.public("GET /api/v1/media/{id}", media.GetAssetHandler(s.media))
	api.public("GET /api/v1/media/{id}/content", media.GetContentHandler(s.media))
	api.admin("DELETE /api/v1/admin/media/{id}", media.DeleteHandler(s.media))

	// ── content / magazine (P4) ──
	api.public("GET /api/v1/topics", content.ListTopicsHandler(s.content))
	api.admin("POST /api/v1/admin/topics", content.CreateTopicHandler(s.content))
	api.public("GET /api/v1/articles", content.ListPublishedArticlesHandler(s.content))
	api.public("GET /api/v1/articles/{slug}", content.GetArticleBySlugHandler(s.content))
	api.public("GET /api/v1/articles/{id}/sources", content.ListArticleSourcesHandler(s.content))
	api.public("GET /api/v1/articles/{id}/products", content.ListRelationsForArticleHandler(s.content))
	api.admin("POST /api/v1/admin/articles", content.CreateArticleHandler(s.content))
	api.admin("POST /api/v1/admin/articles/{id}/submit", content.SubmitForReviewHandler(s.content))
	api.admin("POST /api/v1/admin/articles/{id}/approve", content.ApproveArticleHandler(s.content))
	api.admin("POST /api/v1/admin/articles/{id}/schedule", content.ScheduleArticleHandler(s.content))
	api.admin("POST /api/v1/admin/articles/{id}/publish", content.PublishArticleHandler(s.content))
	api.admin("POST /api/v1/admin/articles/{id}/unpublish", content.UnpublishArticleHandler(s.content))
	api.admin("POST /api/v1/admin/articles/{id}/archive", content.ArchiveArticleHandler(s.content))
	api.admin("POST /api/v1/admin/articles/{id}/reject", content.RejectToDraftHandler(s.content))
	api.admin("POST /api/v1/admin/articles/{id}/sources", content.AddArticleSourceHandler(s.content))
	api.admin("POST /api/v1/admin/articles/{id}/topics", content.LinkArticleTopicHandler(s.content))
	api.admin("POST /api/v1/admin/articles/{id}/products", content.AddProductRelationHandler(s.content))

	// ── ecosystem (P7) ──
	api.public("GET /api/v1/search", ecosystem.SearchHandler(s.ecosystem))
	api.public("GET /api/v1/navigation", ecosystem.NavigationHandler())
}
