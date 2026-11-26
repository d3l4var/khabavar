// KHABAVAR — typed API client.
//
// Every request goes through one place so the base URL, error envelope and JSON shape
// are handled consistently. The API returns snake_case JSON (the contract's convention)
// and amounts in IRR; toman conversion belongs to the presentation layer only
// (ADR-DATA-001), which is why formatToman lives here and nowhere else.
const BASE = import.meta.env.VITE_API_BASE ?? '/api/v1';
export class ApiError extends Error {
    status;
    code;
    constructor(status, code, message) {
        super(message);
        this.status = status;
        this.code = code;
    }
}
async function request(path, init) {
    const res = await fetch(`${BASE}${path}`, {
        headers: { Accept: 'application/json', ...(init?.body ? { 'Content-Type': 'application/json' } : {}) },
        ...init,
    });
    if (!res.ok) {
        let code = 'http_error';
        let message = `request failed (${res.status})`;
        try {
            const body = (await res.json());
            if (body.error)
                message = body.error;
            if (body.code)
                code = body.code;
        }
        catch {
            /* non-JSON error body */
        }
        throw new ApiError(res.status, code, message);
    }
    return (await res.json());
}
// ── catalogue ──
export async function listCategories() {
    return request('/categories');
}
export async function listProductsByCategory(categoryId, page = 1, pageSize = 20) {
    return request(`/categories/${encodeURIComponent(categoryId)}/products?page=${page}&page_size=${pageSize}`);
}
export async function getProduct(id) {
    return request(`/products/${encodeURIComponent(id)}`);
}
export async function searchProducts(q, page = 1, pageSize = 20) {
    return request(`/products/search?q=${encodeURIComponent(q)}&page=${page}&page_size=${pageSize}`);
}
// ── magazine ──
export async function listArticles(page = 1, pageSize = 20) {
    return request(`/articles?page=${page}&page_size=${pageSize}`);
}
export async function getArticle(slug) {
    return request(`/articles/${encodeURIComponent(slug)}`);
}
export async function listTopics() {
    return request('/topics');
}
// ── ecosystem ──
export async function unifiedSearch(q, page = 1, pageSize = 10) {
    return request(`/search?q=${encodeURIComponent(q)}&page=${page}&page_size=${pageSize}`);
}
export async function navigation() {
    return request('/navigation');
}
// ── money (presentation only) ──
// Amounts travel as IRR integers; the UI shows toman (ADR-DATA-001).
export function formatToman(rial) {
    const toman = Math.round(rial / 10);
    return toman.toLocaleString('fa-IR') + ' تومان';
}
