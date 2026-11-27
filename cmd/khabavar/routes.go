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

	"github.com/khabavar/khabavar/internal/modules/audit"
	"github.com/khabavar/khabavar/internal/modules/authorization"
	"github.com/khabavar/khabavar/internal/modules/catalog"
	"github.com/khabavar/khabavar/internal/modules/commerce"
	"github.com/khabavar/khabavar/internal/modules/content"
	"github.com/khabavar/khabavar/internal/modules/ecosystem"
	"github.com/khabavar/khabavar/internal/modules/fulfillment"
	"github.com/khabavar/khabavar/internal/modules/identity"
	"github.com/khabavar/khabavar/internal/modules/inventory"
	"github.com/khabavar/khabavar/internal/modules/media"
	"github.com/khabavar/khabavar/internal/modules/pricing"
	"github.com/khabavar/khabavar/internal/modules/shared"
	"github.com/khabavar/khabavar/internal/modules/suppliers"
	"github.com/khabavar/khabavar/internal/platform/config"
)

// adminPermissions maps each mounted admin route pattern to the seed permission code
// it requires. Codes come from 001_seed_baseline.sql (24 permissions, IAM §۳); the
// server never checks role names (IAM §۱).
var adminPermissions = map[string]string{
	// ── commerce: refunds are FINANCE-exclusive, reconciliation is a finance read ──
	"POST /api/v1/admin/orders/{orderNumber}/refunds": "order.refund.approve",
	"POST /api/v1/admin/return-requests/{id}/approve": "order.manage",
	"POST /api/v1/admin/return-requests/{id}/reject":  "order.manage",
	"POST /api/v1/admin/return-requests/{id}/receive": "order.manage",
	"GET /api/v1/admin/orders/{orderNumber}/refunds":  "order.view",
	"GET /api/v1/admin/orders/{id}/reconcile":         "reports.financial.view",

	// ── suppliers (SUPPLIER_OPERATOR) ──
	"POST /api/v1/admin/suppliers":                           "supplier.manage",
	"PATCH /api/v1/admin/suppliers/{id}":                     "supplier.manage",
	"PATCH /api/v1/admin/suppliers/{id}/status":              "supplier.manage",
	"GET /api/v1/admin/supplier-variants/{id}/price-history": "supplier.manage",
	"POST /api/v1/admin/suppliers/{supplierId}/evaluations":  "supplier.evaluate",
	"GET /api/v1/admin/suppliers/{supplierId}/evaluations":   "supplier.manage",

	// ── inventory (INVENTORY_MANAGER; adjustment is the tighter code) ──
	"POST /api/v1/admin/warehouses":                               "inventory.manage",
	"GET /api/v1/admin/warehouses":                                "inventory.manage",
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
	"POST /api/v1/admin/orders/{id}/fulfillments":           "order.manage",
	"POST /api/v1/admin/fulfillments/{id}/shipments":        "order.manage",
	"POST /api/v1/admin/shipments/{id}/events":              "order.manage",

	// ── media (catalog owners) ──
	"POST /api/v1/admin/media":        "catalog.product.edit",
	"DELETE /api/v1/admin/media/{id}": "catalog.product.edit",

	// ── supply panel (R-07 admin-supply) ──
	"GET /api/v1/admin/stock":            "inventory.manage",
	"POST /api/v1/admin/stock/movements": "inventory.adjust",
	"GET /api/v1/admin/reservations":     "inventory.manage",

	// ── catalogue writes (QC-05, R-04) ──
	"POST /api/v1/admin/products":                     "catalog.product.edit",
	"GET /api/v1/admin/products":                      "catalog.product.view",
	"GET /api/v1/admin/products/{productId}":          "catalog.product.view",
	"PATCH /api/v1/admin/products/{productId}":        "catalog.product.edit",
	"POST /api/v1/admin/products/{productId}/publish": "catalog.product.publish",
	"POST /api/v1/admin/products/{productId}/status":  "catalog.product.publish",
	"POST /api/v1/admin/categories":                   "catalog.attribute.manage",
	"POST /api/v1/admin/variants":                     "catalog.product.edit",

	// ── pricing writes (R-05) ──
	"POST /api/v1/admin/variants/{variantId}/cost-records":  "pricing.cost.manage",
	"GET /api/v1/admin/variants/{variantId}/cost-records":   "pricing.cost.manage",
	"POST /api/v1/admin/variants/{variantId}/price-records": "pricing.price.approve",
	"GET /api/v1/admin/variants/{variantId}/price-records":  "pricing.price.approve",
	"POST /api/v1/admin/price-records/{id}/approve":         "pricing.price.approve",
	"GET /api/v1/admin/variants/{id}/economics":             "reports.financial.view",

	// ── admin reads (contract: /admin/orders, /admin/audit-logs, /admin/users|roles|permissions) ──
	"GET /api/v1/admin/orders":       "order.view",
	"GET /api/v1/admin/audit-logs":   "audit.read",
	"GET /api/v1/admin/users":        "identity.user.manage",
	"POST /api/v1/admin/users":       "identity.user.manage",
	"PATCH /api/v1/admin/users/{id}": "identity.user.manage",
	"GET /api/v1/admin/roles":        "identity.user.manage",
	"GET /api/v1/admin/permissions":  "identity.user.manage",

	// ── content / magazine editorial workflow ──
	"POST /api/v1/admin/topics":                  "content.article.edit",
	"POST /api/v1/admin/articles":                "content.article.edit",
	"PATCH /api/v1/admin/articles/{id}":          "content.article.edit",
	"POST /api/v1/admin/articles/{id}/review":    "content.article.review",
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
	"POST /api/v1/admin/articles/{id}/relations": "content.relation.manage",
	"POST /api/v1/admin/orders/{id}/cancel":      "order.manage",
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
	resolve authorization.SubjectResolver
	mounted map[string]string
}

// newAPIMux builds the mux wrapper. guard must authenticate the caller and check the
// permission code; runAPI passes authorization.Authenticate ∘ RequireAudited.
func newAPIMux(mux *http.ServeMux, guard func(string, http.Handler) http.Handler, resolve authorization.SubjectResolver) *apiMux {
	return &apiMux{mux: mux, guard: guard, resolve: resolve, mounted: map[string]string{}}
}

// public mounts a route that needs no staff permission (storefront reads, health).
func (a *apiMux) public(pattern string, h http.Handler) {
	a.mux.Handle(pattern, h)
	a.mounted[pattern] = ""
}

// publicCustomer mounts a public route that behaves differently for a signed-in customer
// (cart, checkout): the identity is attached when present, and the request is anonymous
// otherwise.
func (a *apiMux) publicCustomer(pattern string, h http.Handler) {
	a.mux.Handle(pattern, authorization.OptionalCustomer(a.resolve, h))
	a.mounted[pattern] = "public+customer"
}

// customer mounts a route that requires a *customer* bearer token (never a staff one),
// and puts the customer id in the request context for the handler.
func (a *apiMux) customer(pattern string, h http.Handler) {
	a.mux.Handle(pattern, authorization.AuthenticateCustomer(a.resolve, h))
	a.mounted[pattern] = "customer"
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
	catalog         *catalog.Service
	pricing         *pricing.Service
	commerce        *commerce.Service
	suppliers       *suppliers.Service
	inventory       *inventory.Service
	fulfillment     *fulfillment.Service
	media           *media.Service
	content         *content.Service
	ecosystem       *ecosystem.Service
	resolve         authorization.SubjectResolver
	customer        identity.CustomerStore
	identity        *identity.Service
	adminLoginStore identity.AdminLoginStore
	cfg             config.Config
	catList         *catalog.AdminListStore
	idem            commerce.IdempotencyStore
	invAdmin        *inventory.AdminStore
	iamAdmin        *identity.AdminStore
	auditList       *audit.ListStore
	orderList       *commerce.OrderListStore
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

	// ── contract aliases (R-07) ────────────────────────────────────────────────
	// The contract names /catalog/* and /magazine/*; the canonical routes above are
	// /products/* and /articles/*. Both are served by the same handler so a client
	// written against the contract works without a second implementation.
	api.public("GET /api/v1/catalog/categories", catalog.CategoryTreeHandler(s.catalog))
	api.public("GET /api/v1/catalog/products", catalog.SearchProductsHandler(s.catalog))
	api.public("GET /api/v1/catalog/products/{id}", catalog.ProductDetailHandler(s.catalog))
	api.public("GET /api/v1/catalog/products/{id}/articles", content.PublishedArticlesForProductHandler(s.content))
	api.public("GET /api/v1/magazine/articles", content.ListPublishedArticlesHandler(s.content))
	api.public("GET /api/v1/magazine/articles/{slug}", content.GetArticleBySlugHandler(s.content))
	api.public("GET /api/v1/magazine/topics", content.ListTopicsHandler(s.content))

	// ── pricing (P5b) ──
	api.public("GET /api/v1/variants/{id}/price", pricing.StorefrontPriceHandler(s.pricing))

	// ── commerce (P4) ──
	// Server-side cart (DB §14.1). Guests are identified by X-Guest-Cart-Token.
	// Customer surface (/me). A staff token is rejected here: the customer API must not
	// be a back door into staff identity.
	api.customer("GET /api/v1/me", identity.MeHandler(s.customer))
	api.customer("GET /api/v1/me/orders", commerce.MyOrdersHandler(s.orderList))
	api.customer("POST /api/v1/cart/merge", commerce.MergeCartHandler(s.commerce))
	// The claim lifecycle is staff-only: a customer files, staff decide.
	api.admin("POST /api/v1/admin/return-requests/{id}/approve", commerce.ApproveReturnHandler(s.commerce))
	api.admin("POST /api/v1/admin/return-requests/{id}/reject", commerce.RejectReturnHandler(s.commerce))
	api.admin("POST /api/v1/admin/return-requests/{id}/receive", commerce.ReceiveReturnHandler(s.commerce))
	api.customer("POST /api/v1/orders/{id}/return-requests",
		commerce.Idempotent(s.idem, "return-request", commerce.CreateReturnRequestHandler(s.commerce)))
	api.customer("GET /api/v1/me/addresses", identity.ListAddressesHandler(s.customer))
	api.customer("POST /api/v1/me/addresses", identity.CreateAddressHandler(s.customer))
	api.customer("PATCH /api/v1/me/addresses/{id}", identity.UpdateAddressHandler(s.customer))
	api.customer("DELETE /api/v1/me/addresses/{id}", identity.DeleteAddressHandler(s.customer))

	api.publicCustomer("GET /api/v1/cart", commerce.CartHandler(s.commerce))
	// Checkout is money-touching, so a retry must not create a second order
	// (ADR-ARCH-008). The wrapper enforces the Idempotency-Key header.
	// Staff login on the API origin (the identity unit has its own copy; the panel talks
	// to whichever host serves /api/v1).
	api.public("POST /api/v1/auth/admin/login", identity.AdminLoginHandler(s.identity, s.adminLoginStore))
	api.public("POST /api/v1/oauth/logout", identity.LogoutHandler())
	api.public("POST /api/v1/payments/webhook", commerce.PaymentWebhookHandler(s.commerce, commerce.PaymentWebhookSecret(s.cfg.PaymentWebhookSecret)))
	api.public("GET /api/v1/payments/return", commerce.PaymentReturnHandler(s.cfg.StorefrontBaseURL))
	api.publicCustomer("POST /api/v1/checkout", commerce.Idempotent(s.idem, "POST /api/v1/checkout", commerce.CheckoutHandler(s.commerce)))
	api.publicCustomer("POST /api/v1/cart/items", commerce.AddCartItemHandler(s.commerce))
	api.publicCustomer("PATCH /api/v1/cart/items/{id}", commerce.UpdateCartItemHandler(s.commerce))
	api.publicCustomer("DELETE /api/v1/cart/items/{id}", commerce.DeleteCartItemHandler(s.commerce))

	api.public("POST /api/v1/orders", commerce.CreateOrderHandler(s.commerce))
	// GET /orders/{id} accepts either the internal UUID or the business order number
	// (KHA-ORD-YYYY-NNNNNN) — the contract exposes /orders/{orderNumber}. The two are
	// one route on purpose: separate /orders/by-number/{n} and /orders/{id}/fulfillment
	// patterns are ambiguous for ServeMux and panic at startup.
	api.public("GET /api/v1/orders/{id}", commerce.GetOrderHandler(s.commerce))
	api.public("POST /api/v1/orders/{id}/payment-intents", commerce.CreatePaymentIntentHandler(s.commerce))
	api.public("POST /api/v1/orders/{id}/confirm-payment", commerce.ConfirmPaymentHandler(s.commerce, commerce.PaymentWebhookSecret(s.cfg.PaymentWebhookSecret)))
	api.public("POST /api/v1/orders/{id}/cancel", commerce.CancelOrderHandler(s.commerce))
	api.admin("POST /api/v1/admin/orders/{orderNumber}/refunds", commerce.CreateRefundHandler(s.commerce))
	api.admin("GET /api/v1/admin/orders/{orderNumber}/refunds", commerce.ListRefundsHandler(s.commerce))
	api.admin("GET /api/v1/admin/orders/{id}/reconcile", commerce.ReconcileTransactionHandler(s.commerce))

	// ── suppliers (P5c) ──
	api.admin("POST /api/v1/admin/suppliers", suppliers.CreateSupplierHandler(s.suppliers))
	api.admin("PATCH /api/v1/admin/suppliers/{id}", suppliers.UpdateSupplierHandler(s.suppliers))
	api.admin("PATCH /api/v1/admin/suppliers/{id}/status", suppliers.UpdateSupplierStatusHandler(s.suppliers))
	api.public("GET /api/v1/variants/{id}/suppliers", suppliers.ListSupplierVariantsHandler(s.suppliers))
	api.admin("GET /api/v1/admin/supplier-variants/{id}/price-history", suppliers.ListPriceHistoryHandler(s.suppliers))
	api.admin("POST /api/v1/admin/suppliers/{supplierId}/evaluations", suppliers.CreateEvaluationHandler(s.suppliers))
	api.admin("GET /api/v1/admin/suppliers/{supplierId}/evaluations", suppliers.ListEvaluationsHandler(s.suppliers))

	// ── inventory (P5d) ──
	api.admin("GET /api/v1/admin/warehouses", inventory.ListWarehousesHandler(s.invAdmin))
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
	api.admin("POST /api/v1/admin/orders/{id}/fulfillments", fulfillment.CreateOrderFulfillmentHandler(s.fulfillment))
	api.admin("POST /api/v1/admin/fulfillments/{id}/shipments", fulfillment.CreateShipmentHandler(s.fulfillment))
	api.admin("POST /api/v1/admin/shipments/{id}/events", fulfillment.RecordShipmentEventHandler(s.fulfillment))
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
	api.admin("PATCH /api/v1/admin/articles/{id}", content.UpdateArticleHandler(s.content))
	api.admin("POST /api/v1/admin/articles/{id}/review", content.ReviewArticleHandler(s.content))
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
	// Contract aliases (R-07): same handlers, contract path names.
	api.admin("POST /api/v1/admin/articles/{id}/relations", content.AddProductRelationHandler(s.content))
	api.admin("POST /api/v1/admin/orders/{id}/cancel", commerce.CancelOrderHandler(s.commerce))

	// ── supply panel (R-07 admin-supply) ──
	api.admin("GET /api/v1/admin/stock", inventory.ListStockHandler(s.invAdmin))
	api.admin("POST /api/v1/admin/stock/movements", inventory.RecordMovementHandler(s.inventory))
	api.admin("GET /api/v1/admin/reservations", inventory.ListReservationsHandler(s.invAdmin))

	// ── catalogue writes (QC-05) ──
	api.admin("POST /api/v1/admin/products", catalog.CreateProductHandler(s.catalog))
	api.admin("GET /api/v1/admin/products", catalog.ListProductsHandler(s.catList))
	api.admin("GET /api/v1/admin/products/{productId}", catalog.AdminProductDetailHandler(s.catalog))
	api.admin("PATCH /api/v1/admin/products/{productId}", catalog.UpdateProductHandler(s.catalog))
	api.admin("POST /api/v1/admin/products/{productId}/publish", catalog.PublishProductHandler(s.catalog))
	api.admin("POST /api/v1/admin/products/{productId}/status", catalog.ProductStatusHandler(s.catalog))
	api.admin("POST /api/v1/admin/categories", catalog.CreateCategoryHandler(s.catalog))
	api.admin("POST /api/v1/admin/variants", catalog.CreateVariantHandler(s.catalog))

	// ── pricing writes (R-05) ──
	api.admin("POST /api/v1/admin/variants/{variantId}/cost-records", pricing.CreateCostRecordHandler(s.pricing))
	api.admin("GET /api/v1/admin/variants/{variantId}/cost-records", pricing.ListCostRecordsHandler(s.pricing))
	api.admin("POST /api/v1/admin/variants/{variantId}/price-records", pricing.CreatePriceRecordHandler(s.pricing))
	api.admin("GET /api/v1/admin/variants/{variantId}/price-records", pricing.ListPriceRecordsHandler(s.pricing))
	api.admin("POST /api/v1/admin/price-records/{id}/approve", pricing.ApprovePriceRecordHandler(s.pricing))
	api.admin("GET /api/v1/admin/variants/{id}/economics", pricing.UnitEconomicsHandler(s.pricing))

	// ── admin reads ──
	api.admin("GET /api/v1/admin/orders", commerce.ListOrdersHandler(s.orderList))
	api.admin("GET /api/v1/admin/audit-logs", audit.ListHandler(s.auditList))
	api.admin("GET /api/v1/admin/users", identity.AdminUsersHandler(s.iamAdmin))
	api.admin("POST /api/v1/admin/users", identity.CreateAdminUserHandler(s.iamAdmin))
	api.admin("PATCH /api/v1/admin/users/{id}", identity.UpdateAdminUserHandler(s.iamAdmin))
	api.admin("GET /api/v1/admin/roles", identity.RolesHandler(s.iamAdmin))
	api.admin("GET /api/v1/admin/permissions", identity.PermissionsHandler(s.iamAdmin))

	// ── ecosystem (P7) ──
	api.public("GET /api/v1/search", ecosystem.SearchHandler(s.ecosystem))
	api.public("GET /api/v1/navigation", ecosystem.NavigationHandler())
}
