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
  status: string
  published_at?: string
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

// ── money (presentation only) ──
// Amounts travel as IRR integers; the UI shows toman (ADR-DATA-001).
export function formatToman(rial: number): string {
  const toman = Math.round(rial / 10)
  return toman.toLocaleString('fa-IR') + ' تومان'
}
