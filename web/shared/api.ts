// KHABAVAR — typed API client.
//
// Every request goes through one place so the base URL, error envelope and JSON shape
// are handled consistently. The API returns snake_case JSON (the contract's convention)
// and amounts in IRR; toman conversion belongs to the presentation layer only
// (ADR-DATA-001), which is why formatToman lives here and nowhere else.

const BASE = import.meta.env.VITE_API_BASE ?? '/api/v1'

export type Product = {
  id: string
  title: string
  slug: string
  short_description?: string
  description?: string
  sleep_relevance?: string
  status: string
  variants?: Variant[]
}

export type Variant = {
  id: string
  product_id: string
  sku: string
  status: string
}

export type Article = {
  id: string
  title: string
  slug: string
  summary?: string
  body?: string
  language?: string
  source_type?: string
  status: string
  published_at?: string
  created_at?: string
  updated_at?: string
}

export type Topic = { id: string; name: string; slug: string; sort_order: number }

export type SearchHit = {
  type: 'product' | 'article'
  id: string
  title: string
  slug: string
  snippet?: string
}

export type Page<T> = { items: T[]; page: { page: number; page_size: number; total: number; total_pages: number } }

export class ApiError extends Error {
  constructor(
    readonly status: number,
    readonly code: string,
    message: string,
  ) {
    super(message)
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    headers: { Accept: 'application/json', ...(init?.body ? { 'Content-Type': 'application/json' } : {}) },
    ...init,
  })
  if (!res.ok) {
    let code = 'http_error'
    let message = `request failed (${res.status})`
    try {
      const body = (await res.json()) as { error?: string; code?: string }
      if (body.error) message = body.error
      if (body.code) code = body.code
    } catch {
      /* non-JSON error body */
    }
    throw new ApiError(res.status, code, message)
  }
  return (await res.json()) as T
}

// ── catalogue ──
export async function listCategories() {
  return request<{ data: unknown[] }>('/categories')
}

export async function listProductsByCategory(categoryId: string, page = 1, pageSize = 20) {
  return request<{ data: Product[]; total: number; page: number; page_size: number }>(
    `/categories/${encodeURIComponent(categoryId)}/products?page=${page}&page_size=${pageSize}`,
  )
}

export async function getProduct(id: string) {
  return request<{ data: Product }>(`/products/${encodeURIComponent(id)}`)
}

export async function searchProducts(q: string, page = 1, pageSize = 20) {
  return request<{ data: { Product: Product; Snippet: string }[]; total: number }>(
    `/products/search?q=${encodeURIComponent(q)}&page=${page}&page_size=${pageSize}`,
  )
}

export type VariantPrice = {
  variant_id: string
  selling_price: number
  discount: number
  net_revenue: number
  currency: string
}

// Storefront price: the confidential cost/economics fields are never exposed here
// (R-05 §۲۴). A variant without an approved price answers 404, which the UI shows as
// "not for sale" rather than as an error.
export async function getVariantPrice(variantId: string) {
  return request<{ data: VariantPrice }>(`/variants/${encodeURIComponent(variantId)}/price`)
}

// ── orders ──
export type OrderItemInput = {
  product_id: string
  variant_id: string
  sku_snapshot: string
  title_snapshot: string
  quantity: number
  unit_price: number
  discount_amount: number
  tax_amount: number
  supply_type: 'OWNED_INVENTORY' | 'SUPPLIER_SOURCED'
}

export type OrderInput = {
  customer_id?: string
  guest_phone?: string
  order_number?: string
  currency?: string
  items: OrderItemInput[]
  shipping_address: Record<string, string>
  discount_total: number
  shipping_total: number
  tax_total: number
}

export async function createOrder(input: OrderInput) {
  return request<{ id: string }>('/orders', { method: 'POST', body: JSON.stringify(input) })
}

export async function getOrderByNumber(number: string) {
  return request<{ id: string; order_number: string; status: string; grand_total: number; paid_total: number }>(
    `/orders/${encodeURIComponent(number)}`,
  )
}

// ── magazine ──
export async function listArticlesByTopic(topicId: string, page = 1, pageSize = 20) {
  return request<{ data: Article[]; total: number }>(
    `/articles?topic=${encodeURIComponent(topicId)}&page=${page}&page_size=${pageSize}`,
  )
}

export async function listArticles(page = 1, pageSize = 20) {
  return request<{ data: Article[]; total: number }>(`/articles?page=${page}&page_size=${pageSize}`)
}

export async function getArticle(slug: string) {
  return request<{ data: Article }>(`/articles/${encodeURIComponent(slug)}`)
}

export async function listTopics() {
  return request<{ data: Topic[] }>('/topics')
}

// ── ecosystem ──
export async function unifiedSearch(q: string, page = 1, pageSize = 10) {
  return request<{ items: SearchHit[]; product_total: number; article_total: number }>(
    `/search?q=${encodeURIComponent(q)}&page=${page}&page_size=${pageSize}`,
  )
}

export async function navigation() {
  return request<{ items: { label: string; href: string; type: string }[] }>('/navigation')
}

// ── admin (staff) ─────────────────────────────────────────────────────────────
// Every admin call carries the Bearer token. The panel stores it in sessionStorage:
// the OIDC Authorization-Code + PKCE client is a later step, and until then the
// operator pastes a session token. The API enforces the permission per route, so a
// token without the code gets a 403 — the UI shows that verbatim.

const TOKEN_KEY = 'kh.admin.token'

export function adminToken(): string {
  return sessionStorage.getItem(TOKEN_KEY) ?? ''
}

export function setAdminToken(token: string) {
  if (token) sessionStorage.setItem(TOKEN_KEY, token)
  else sessionStorage.removeItem(TOKEN_KEY)
}

async function adminRequest<T>(path: string, init?: RequestInit): Promise<T> {
  const token = adminToken()
  const res = await fetch(`${BASE}${path}`, {
    ...init,
    headers: {
      Accept: 'application/json',
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
      ...(init?.body ? { 'Content-Type': 'application/json' } : {}),
      ...(init?.headers as Record<string, string> | undefined),
    },
  })
  if (!res.ok) {
    let code = 'http_error'
    let message = res.status === 401 ? 'توکن نامعتبر یا منقضی است.' : res.status === 403 ? 'این عملیات به مجوز لازم نیاز دارد.' : `خطای ${res.status}`
    try {
      const body = (await res.json()) as { error?: string; code?: string }
      if (body.code) code = body.code
      if (body.error && res.status < 500) message = body.error
    } catch {
      /* non-JSON */
    }
    throw new ApiError(res.status, code, message)
  }
  return (await res.json()) as T
}

// Staff login: email + password against the identity unit, which answers an opaque
// session token. The token is what every /admin/* call sends as Bearer.
export async function adminLogin(email: string, password: string) {
  const res = await fetch(`${BASE}/auth/admin/login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
    body: JSON.stringify({ email, password }),
  })
  if (!res.ok) {
    const status = res.status
    const message =
      status === 401 ? 'ایمیل یا گذرواژه نادرست است.' : status === 403 ? 'این حساب فعال نیست.' : `خطای ${status}`
    throw new ApiError(status, 'login_failed', message)
  }
  const body = (await res.json()) as { access_token: string }
  setAdminToken(body.access_token)
  return body.access_token
}

export type AdminOrder = {
  id: string
  order_number: string
  status: string
  currency: string
  grand_total: number
  paid_total: number
  refunded_total: number
  guest_phone?: string
  created_at: string
}

export type AdminProductRow = {
  id: string
  title: string
  slug: string
  status: string
  sleep_relevance?: string
  spec_source?: string
  publish_gate_satisfied: boolean
  updated_at: string
}

export type StockRow = {
  variant_id: string
  sku: string
  warehouse_code: string
  on_hand_quantity: number
  reserved_quantity: number
  available_quantity: number
  updated_at: string
}

export type AuditRow = {
  id: string
  actor_type: string
  actor_id: string
  action: string
  entity_type: string
  entity_id: string
  new_values?: unknown
  created_at: string
}

export type RoleRow = { code: string; name: string; description: string; permissions: string[] }

export async function adminListOrders(status = '', page = 1, pageSize = 20) {
  return adminRequest<{ data: AdminOrder[]; total: number }>(
    `/admin/orders?status=${encodeURIComponent(status)}&page=${page}&page_size=${pageSize}`,
  )
}

export async function adminListProducts(status = '', q = '', page = 1, pageSize = 20) {
  return adminRequest<{ data: AdminProductRow[]; total: number }>(
    `/admin/products?status=${encodeURIComponent(status)}&q=${encodeURIComponent(q)}&page=${page}&page_size=${pageSize}`,
  )
}

export async function adminPublishProduct(id: string) {
  return adminRequest<{ status: string }>(`/admin/products/${encodeURIComponent(id)}/publish`, { method: 'POST' })
}

export async function adminPatchProduct(id: string, patch: Record<string, unknown>) {
  return adminRequest<AdminProductRow>(`/admin/products/${encodeURIComponent(id)}`, {
    method: 'PATCH',
    body: JSON.stringify(patch),
  })
}

export type AdminVariant = { id: string; product_id: string; sku: string; status: string }
export type AdminProductDetail = AdminProductRow & {
  short_description?: string
  description?: string
  spec_last_reviewed_at?: string
  variants?: AdminVariant[]
}

export async function adminGetProduct(id: string) {
  return adminRequest<AdminProductDetail>(`/admin/products/${encodeURIComponent(id)}`)
}

export async function adminCreateProduct(input: {
  title: string
  slug: string
  short_description?: string
  description?: string
}) {
  return adminRequest<{ id: string }>('/admin/products', { method: 'POST', body: JSON.stringify(input) })
}

export async function adminCreateVariant(input: { product_id: string; sku: string; status?: string }) {
  return adminRequest<{ id: string }>('/admin/variants', { method: 'POST', body: JSON.stringify(input) })
}

export type CostRecordInput = {
  supply_type: string
  purchase_cost: number
  inbound_freight?: number
  inspection_cost?: number
  packaging_cost?: number
  fulfillment_cost?: number
}

export async function adminCreateCostRecord(variantId: string, input: CostRecordInput) {
  return adminRequest<{ id: string }>(`/admin/variants/${encodeURIComponent(variantId)}/cost-records`, {
    method: 'POST',
    body: JSON.stringify(input),
  })
}

export type PriceRecordInput = { selling_price: number; currency?: string }

export async function adminCreatePriceRecord(variantId: string, input: PriceRecordInput) {
  return adminRequest<{ id: string; warning?: string }>(`/admin/variants/${encodeURIComponent(variantId)}/price-records`, {
    method: 'POST',
    body: JSON.stringify(input),
  })
}

// price_records has no status column: approval is expressed by approved_by (R-05 §۱۹).
export type AdminPriceRecord = {
  id: string
  selling_price: number
  currency: string
  approved_by?: string
  valid_from?: string
}

export async function adminListPriceRecords(variantId: string) {
  return adminRequest<{ data: AdminPriceRecord[] }>(`/admin/variants/${encodeURIComponent(variantId)}/price-records`)
}

export async function adminApprovePriceRecord(id: string) {
  return adminRequest<{ status: string }>(`/admin/price-records/${encodeURIComponent(id)}/approve`, { method: 'POST' })
}

export async function adminListStock(q = '', page = 1, pageSize = 20) {
  return adminRequest<{ items: StockRow[]; page: { total: number } }>(
    `/admin/stock?q=${encodeURIComponent(q)}&page=${page}&page_size=${pageSize}`,
  )
}

export async function adminRecordMovement(input: {
  warehouse_id: string
  variant_id: string
  movement_type: string
  quantity: number
  note?: string
}) {
  return adminRequest<{ id: string }>('/admin/stock/movements', { method: 'POST', body: JSON.stringify(input) })
}

export async function adminListAudit(action = '', page = 1, pageSize = 20) {
  return adminRequest<{ data: AuditRow[]; total: number }>(
    `/admin/audit-logs?action=${encodeURIComponent(action)}&page=${page}&page_size=${pageSize}`,
  )
}

export async function adminListRoles() {
  return adminRequest<{ data: RoleRow[] }>('/admin/roles')
}

export async function adminListWarehouses() {
  return adminRequest<{ data: unknown[] }>('/admin/warehouses').catch(() => ({ data: [] }))
}

// ── money (presentation only) ──
// Amounts travel as IRR integers; the UI shows toman (ADR-DATA-001).
export function formatToman(rial: number): string {
  const toman = Math.round(rial / 10)
  return toman.toLocaleString('fa-IR') + ' تومان'
}
