// KHABAVAR — admin panel (admin.khabavar.ir).
//
// The API enforces RBAC per route (authorization.Require + the seeded permission
// matrix), so this app never decides permissions itself: it shows whatever the API
// allows and surfaces a 403 verbatim. That keeps the single source of truth in the
// seed (IAM §۱) instead of duplicating the matrix in the client.
//
// Auth: the OIDC Authorization-Code + PKCE client is a later step, so for now the
// operator pastes a session token; it is kept in sessionStorage and sent as Bearer.
import { useEffect, useMemo, useState } from 'react'
import { createRoot } from 'react-dom/client'
import { Badge, Button, Card, Container, Footer, Header, StateBlock, useAsync } from '../../shared/components'
import {
  adminListAudit,
  adminListOrders,
  beginOIDCLogin,
  completeOIDCLogin,
  adminListProducts,
  adminCreateProduct,
  adminListRoles,
  adminListStock,
  adminPatchProduct,
  adminPublishProduct,
  adminRecordMovement,
  adminToken,
  formatToman,
  setAdminToken,
  type AdminOrder,
  type AdminProductRow,
} from '../../shared/api'
import '../../shared/tokens.css'
import { ProductDetailScreen } from './ProductDetail'

const NAV = [
  { label: 'سفارش‌ها', href: '#/orders' },
  { label: 'محصولات', href: '#/products' },
  { label: 'انبار', href: '#/stock' },
  { label: 'Audit', href: '#/audit' },
  { label: 'نقش‌ها', href: '#/roles' },
]

function useHashRoute() {
  const [hash, setHash] = useState(() => window.location.hash || '#/orders')
  useEffect(() => {
    const onChange = () => setHash(window.location.hash || '#/orders')
    window.addEventListener('hashchange', onChange)
    return () => window.removeEventListener('hashchange', onChange)
  }, [])
  return hash
}

function LoginScreen({ initialError }: { initialError?: string | null }) {
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(initialError ?? null)

  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError(null)
    setBusy(true)
    try {
      // Full-page navigation: authorize authenticates with the HttpOnly cookie the
      // login just set, which a fetch cannot carry across a redirect.
      window.location.href = await beginOIDCLogin(email, password)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'ورود ناموفق بود.')
    } finally {
      setBusy(false)
    }
  }

  return (
    <Container>
      <h1 style={{ marginTop: 'var(--kh-space-6)' }}>ورود به پنل مدیریت</h1>
      <p style={{ color: 'var(--kh-text-muted)' }}>
        ورود با ایمیل و گذرواژهٔ سازمانی. توکن نشست در همین تب نگه داشته می‌شود.
      </p>
      <form onSubmit={submit} style={{ display: 'grid', gap: 'var(--kh-space-3)', maxWidth: '420px' }}>
        <label>
          ایمیل
          <input
            type="email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            required
            autoComplete="username"
            style={{ width: '100%', padding: 'var(--kh-space-3)', marginTop: 'var(--kh-space-1)' }}
          />
        </label>
        <label>
          گذرواژه
          <input
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            required
            autoComplete="current-password"
            style={{ width: '100%', padding: 'var(--kh-space-3)', marginTop: 'var(--kh-space-1)' }}
          />
        </label>
        {error ? <StateBlock kind="error" message={error} /> : null}
        <Button type="submit" disabled={busy}>
          {busy ? 'در حال ورود…' : 'ورود'}
        </Button>
      </form>
    </Container>
  )
}

function OrdersScreen() {
  const [status, setStatus] = useState('')
  const orders = useAsync(() => adminListOrders(status), [status])
  return (
    <Container>
      <h1 style={{ marginTop: 'var(--kh-space-5)' }}>سفارش‌ها</h1>
      <div style={{ display: 'flex', gap: 'var(--kh-space-2)', marginBottom: 'var(--kh-space-4)', flexWrap: 'wrap' }}>
        {['', 'PENDING_PAYMENT', 'PAID', 'CANCELLED', 'CLOSED'].map((s) => (
          <Button key={s || 'all'} variant={status === s ? 'primary' : 'ghost'} onClick={() => setStatus(s)}>
            {s || 'همه'}
          </Button>
        ))}
      </div>
      {orders.loading ? <StateBlock kind="loading" /> : null}
      {orders.error ? <StateBlock kind="error" message={orders.error} onRetry={orders.retry} /> : null}
      {orders.data && (orders.data.data ?? []).length === 0 ? <StateBlock kind="empty" message="سفارشی نیست." /> : null}
      {orders.data && (orders.data.data ?? []).length > 0 ? (
        <table style={{ width: '100%', borderCollapse: 'collapse' }}>
          <thead>
            <tr style={{ textAlign: 'start', color: 'var(--kh-text-muted)', fontSize: 'var(--kh-text-sm)' }}>
              <th>شماره</th>
              <th>وضعیت</th>
              <th>مبلغ</th>
              <th>پرداخت‌شده</th>
              <th>بازپرداخت</th>
            </tr>
          </thead>
          <tbody>
            {(orders.data.data ?? []).map((o: AdminOrder) => (
              <tr key={o.id} style={{ borderTop: '1px solid var(--kh-border)' }}>
                <td style={{ padding: 'var(--kh-space-2)' }}>{o.order_number}</td>
                <td>
                  <Badge tone={o.status === 'PAID' ? 'success' : o.status === 'CANCELLED' ? 'error' : 'neutral'}>{o.status}</Badge>
                </td>
                <td>{formatToman(o.grand_total)}</td>
                <td>{formatToman(o.paid_total)}</td>
                <td>{formatToman(o.refunded_total)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      ) : null}
    </Container>
  )
}

function ProductsScreen() {
  const [q, setQ] = useState('')
  const [submitted, setSubmitted] = useState('')
  const products = useAsync(() => adminListProducts('', submitted), [submitted])
  const [busy, setBusy] = useState<string | null>(null)
  const [msg, setMsg] = useState<string | null>(null)

  const publish = async (id: string) => {
    setBusy(id)
    setMsg(null)
    try {
      await adminPublishProduct(id)
      setMsg('محصول منتشر شد.')
      products.retry()
    } catch (err) {
      setMsg(err instanceof Error ? err.message : 'انتشار ناموفق بود.')
    } finally {
      setBusy(null)
    }
  }

  const fillGate = async (id: string) => {
    setBusy(id)
    setMsg(null)
    try {
      await adminPatchProduct(id, {
        sleep_relevance: 'SLEEP_RELATED',
        spec_source: 'برگه فنی سازنده',
        spec_last_reviewed_at: new Date().toISOString(),
      })
      setMsg('فیلدهای دروازهٔ انتشار ثبت شد.')
      products.retry()
    } catch (err) {
      setMsg(err instanceof Error ? err.message : 'ویرایش ناموفق بود.')
    } finally {
      setBusy(null)
    }
  }

  return (
    <Container>
      <h1 style={{ marginTop: 'var(--kh-space-5)' }}>محصولات</h1>
      <form
        onSubmit={(e) => {
          e.preventDefault()
          setSubmitted(q.trim())
        }}
        style={{ display: 'flex', gap: 'var(--kh-space-2)', margin: 'var(--kh-space-3) 0' }}
      >
        <input
          value={q}
          onChange={(e) => setQ(e.target.value)}
          placeholder="عنوان یا اسلاگ…"
          aria-label="جست‌وجوی محصول"
          style={{ flex: 1, padding: 'var(--kh-space-3)', borderRadius: 'var(--kh-radius-sm)', border: '1px solid var(--kh-border)', background: 'var(--kh-surface)', color: 'var(--kh-text)', font: 'inherit' }}
        />
        <Button type="submit">جست‌وجو</Button>
      </form>

      <NewProductForm onCreated={(id) => { window.location.hash = `#/product/${encodeURIComponent(id)}` }} />

      {msg ? <p style={{ color: 'var(--kh-text-muted)' }}>{msg}</p> : null}
      {products.loading ? <StateBlock kind="loading" /> : null}
      {products.error ? <StateBlock kind="error" message={products.error} onRetry={products.retry} /> : null}
      {products.data && (products.data.data ?? []).length === 0 ? <StateBlock kind="empty" message="محصولی نیست." /> : null}
      {products.data && (products.data.data ?? []).length > 0 ? (
        <div style={{ display: 'grid', gap: 'var(--kh-space-3)' }}>
          {(products.data.data ?? []).map((p: AdminProductRow) => (
            <Card key={p.id}>
              <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--kh-space-3)', flexWrap: 'wrap' }}>
                <a href={`#/product/${encodeURIComponent(p.id)}`} style={{ color: 'var(--kh-text)' }}>
                  <strong>{p.title}</strong>
                </a>
                <Badge tone={p.status === 'PUBLISHED' ? 'success' : p.status === 'PAUSED' ? 'warning' : 'neutral'}>{p.status}</Badge>
                <Badge tone={p.publish_gate_satisfied ? 'success' : 'error'}>
                  {p.publish_gate_satisfied ? 'دروازهٔ انتشار کامل' : 'دروازهٔ انتشار ناقص'}
                </Badge>
                <span style={{ color: 'var(--kh-text-muted)', fontSize: 'var(--kh-text-sm)' }}>{p.slug}</span>
                <div style={{ marginInlineStart: 'auto', display: 'flex', gap: 'var(--kh-space-2)' }}>
                  {!p.publish_gate_satisfied ? (
                    <Button variant="ghost" onClick={() => fillGate(p.id)} disabled={busy === p.id}>
                      تکمیل دروازه
                    </Button>
                  ) : null}
                  {p.status !== 'PUBLISHED' ? (
                    <Button onClick={() => publish(p.id)} disabled={busy === p.id || !p.publish_gate_satisfied}>
                      انتشار
                    </Button>
                  ) : null}
                </div>
              </div>
            </Card>
          ))}
        </div>
      ) : null}
    </Container>
  )
}

function StockScreen() {
  const [q, setQ] = useState('')
  const [submitted, setSubmitted] = useState('')
  const stock = useAsync(() => adminListStock(submitted), [submitted])
  const [form, setForm] = useState({ warehouse_id: '', variant_id: '', movement_type: 'RECEIPT', quantity: 1 })
  const [msg, setMsg] = useState<string | null>(null)

  return (
    <Container>
      <h1 style={{ marginTop: 'var(--kh-space-5)' }}>انبار</h1>
      <form
        onSubmit={(e) => {
          e.preventDefault()
          setSubmitted(q.trim())
        }}
        style={{ display: 'flex', gap: 'var(--kh-space-2)', margin: 'var(--kh-space-3) 0' }}
      >
        <input
          value={q}
          onChange={(e) => setQ(e.target.value)}
          placeholder="SKU یا عنوان…"
          aria-label="جست‌وجوی موجودی"
          style={{ flex: 1, padding: 'var(--kh-space-3)', borderRadius: 'var(--kh-radius-sm)', border: '1px solid var(--kh-border)', background: 'var(--kh-surface)', color: 'var(--kh-text)', font: 'inherit' }}
        />
        <Button type="submit">جست‌وجو</Button>
      </form>

      {stock.loading ? <StateBlock kind="loading" /> : null}
      {stock.error ? <StateBlock kind="error" message={stock.error} onRetry={stock.retry} /> : null}
      {stock.data && (stock.data.items ?? []).length === 0 ? <StateBlock kind="empty" message="موجودی ثبت نشده است." /> : null}
      {stock.data && (stock.data.items ?? []).length > 0 ? (
        <table style={{ width: '100%', borderCollapse: 'collapse', marginBottom: 'var(--kh-space-5)' }}>
          <thead>
            <tr style={{ textAlign: 'start', color: 'var(--kh-text-muted)', fontSize: 'var(--kh-text-sm)' }}>
              <th>SKU</th>
              <th>انبار</th>
              <th>موجود</th>
              <th>رزرو</th>
              <th>قابل فروش</th>
            </tr>
          </thead>
          <tbody>
            {(stock.data.items ?? []).map((r) => (
              <tr key={`${r.warehouse_code}-${r.variant_id}`} style={{ borderTop: '1px solid var(--kh-border)' }}>
                <td style={{ padding: 'var(--kh-space-2)' }}>{r.sku}</td>
                <td>{r.warehouse_code}</td>
                <td>{r.on_hand_quantity}</td>
                <td>{r.reserved_quantity}</td>
                <td>
                  <Badge tone={r.available_quantity > 0 ? 'success' : 'warning'}>{r.available_quantity}</Badge>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      ) : null}

      <h2>ثبت حرکت موجودی</h2>
      <form
        onSubmit={async (e) => {
          e.preventDefault()
          setMsg(null)
          try {
            await adminRecordMovement({
              warehouse_id: form.warehouse_id,
              variant_id: form.variant_id,
              movement_type: form.movement_type,
              quantity: Number(form.quantity),
            })
            setMsg('حرکت ثبت شد.')
            stock.retry()
          } catch (err) {
            setMsg(err instanceof Error ? err.message : 'ثبت حرکت ناموفق بود.')
          }
        }}
        style={{ display: 'grid', gap: 'var(--kh-space-2)', maxWidth: '520px' }}
      >
        <input placeholder="warehouse_id" value={form.warehouse_id} onChange={(e) => setForm({ ...form, warehouse_id: e.target.value })} required style={{ padding: 'var(--kh-space-2)' }} />
        <input placeholder="variant_id" value={form.variant_id} onChange={(e) => setForm({ ...form, variant_id: e.target.value })} required style={{ padding: 'var(--kh-space-2)' }} />
        <select value={form.movement_type} onChange={(e) => setForm({ ...form, movement_type: e.target.value })} style={{ padding: 'var(--kh-space-2)' }}>
          {['RECEIPT', 'RETURN', 'ADJUSTMENT', 'DAMAGE', 'TRANSFER'].map((t) => (
            <option key={t} value={t}>
              {t}
            </option>
          ))}
        </select>
        <input type="number" value={form.quantity} onChange={(e) => setForm({ ...form, quantity: Number(e.target.value) })} required style={{ padding: 'var(--kh-space-2)' }} />
        <Button type="submit">ثبت</Button>
      </form>
      {msg ? <p style={{ color: 'var(--kh-text-muted)' }}>{msg}</p> : null}
    </Container>
  )
}

function AuditScreen() {
  const audit = useAsync(() => adminListAudit(), [])
  return (
    <Container>
      <h1 style={{ marginTop: 'var(--kh-space-5)' }}>Audit Log</h1>
      {audit.loading ? <StateBlock kind="loading" /> : null}
      {audit.error ? <StateBlock kind="error" message={audit.error} onRetry={audit.retry} /> : null}
      {audit.data && (audit.data.data ?? []).length === 0 ? <StateBlock kind="empty" message="رکوردی نیست." /> : null}
      {audit.data && (audit.data.data ?? []).length > 0 ? (
        <ul style={{ listStyle: 'none', padding: 0, display: 'grid', gap: 'var(--kh-space-2)' }}>
          {(audit.data.data ?? []).map((e) => (
            <li key={e.id}>
              <Card>
                <div style={{ display: 'flex', gap: 'var(--kh-space-3)', flexWrap: 'wrap', alignItems: 'center' }}>
                  <Badge>{e.action}</Badge>
                  <span style={{ fontSize: 'var(--kh-text-sm)' }}>{e.entity_type}</span>
                  <span style={{ color: 'var(--kh-text-muted)', fontSize: 'var(--kh-text-sm)' }}>{e.created_at}</span>
                </div>
              </Card>
            </li>
          ))}
        </ul>
      ) : null}
    </Container>
  )
}

function RolesScreen() {
  const roles = useAsync(() => adminListRoles(), [])
  return (
    <Container>
      <h1 style={{ marginTop: 'var(--kh-space-5)' }}>نقش‌ها و مجوزها</h1>
      {roles.loading ? <StateBlock kind="loading" /> : null}
      {roles.error ? <StateBlock kind="error" message={roles.error} onRetry={roles.retry} /> : null}
      {roles.data && (roles.data.data ?? []).length > 0 ? (
        <div style={{ display: 'grid', gap: 'var(--kh-space-3)' }}>
          {(roles.data.data ?? []).map((r) => (
            <Card key={r.code}>
              <strong>
                {r.code} — {r.name}
              </strong>
              <p style={{ margin: 0, color: 'var(--kh-text-muted)', fontSize: 'var(--kh-text-sm)' }}>{r.description}</p>
              <p style={{ margin: 0, fontSize: 'var(--kh-text-sm)' }}>{r.permissions.length} مجوز: {r.permissions.join(' · ')}</p>
            </Card>
          ))}
        </div>
      ) : null}
    </Container>
  )
}

function App() {
  const hash = useHashRoute()
  const [authed, setAuthed] = useState(() => Boolean(adminToken()))
  const [booting, setBooting] = useState(() => new URL(window.location.href).searchParams.has('code'))
  const [bootError, setBootError] = useState<string | null>(null)

  useEffect(() => {
    if (!booting) return
    completeOIDCLogin()
      .then((ok) => ok && setAuthed(true))
      .catch((err: unknown) => setBootError(err instanceof Error ? err.message : 'ورود ناموفق بود.'))
      .finally(() => setBooting(false))
  }, [booting])
  const route = useMemo(() => {
    const segs = hash.replace(/^#\/?/, '').split('/').filter(Boolean)
    return { name: segs[0] || 'orders', param: segs[1] ? decodeURIComponent(segs[1]) : '' }
  }, [hash])

  if (!authed) {
    return (
      <div style={{ minHeight: '100%', display: 'flex', flexDirection: 'column' }}>
        <Header items={[]} />
        <main style={{ flex: 1 }}>
          <LoginScreen initialError={bootError} />
        </main>
        <Footer />
      </div>
    )
  }

  let screen: JSX.Element
  switch (route.name) {
    case 'products':
      screen = <ProductsScreen />
      break
    case 'product':
      screen = <ProductDetailScreen id={route.param} />
      break
    case 'stock':
      screen = <StockScreen />
      break
    case 'audit':
      screen = <AuditScreen />
      break
    case 'roles':
      screen = <RolesScreen />
      break
    default:
      screen = <OrdersScreen />
  }

  return (
    <div style={{ minHeight: '100%', display: 'flex', flexDirection: 'column' }}>
      <Header items={NAV} />
      <main style={{ flex: 1 }}>{screen}</main>
      <div style={{ padding: '0 var(--kh-space-4)' }}>
        <Button
          variant="ghost"
          onClick={() => {
            setAdminToken('')
            setAuthed(false)
          }}
        >
          خروج
        </Button>
      </div>
      <Footer />
    </div>
  )
}

createRoot(document.getElementById('root')!).render(<App />)

// NewProductForm creates a DRAFT product and jumps to its detail page, where the
// publish gate, categories, variants and pricing live. The slug is derived from the
// title when left blank, and the API rejects a duplicate.
function NewProductForm({ onCreated }: { onCreated: (id: string) => void }) {
  const [open, setOpen] = useState(false)
  const [form, setForm] = useState({ title: '', slug: '', short_description: '' })
  const [msg, setMsg] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  const field: React.CSSProperties = {
    width: '100%',
    padding: 'var(--kh-space-2)',
    borderRadius: 'var(--kh-radius-sm)',
    border: '1px solid var(--kh-border)',
    background: 'var(--kh-surface)',
    color: 'var(--kh-text)',
    font: 'inherit',
  }

  if (!open) {
    return (
      <div style={{ margin: 'var(--kh-space-3) 0' }}>
        <Button onClick={() => setOpen(true)}>محصول جدید</Button>
      </div>
    )
  }

  return (
    <form
      onSubmit={async (e) => {
        e.preventDefault()
        setBusy(true)
        setMsg(null)
        try {
          const slug =
            form.slug.trim() ||
            form.title
              .trim()
              .toLowerCase()
              .replace(/[^\p{L}\p{N}]+/gu, '-')
              .replace(/^-|-$/g, '')
          const created = await adminCreateProduct({
            title: form.title.trim(),
            slug,
            short_description: form.short_description.trim() || undefined,
          })
          setForm({ title: '', slug: '', short_description: '' })
          setOpen(false)
          onCreated(created.id)
        } catch (err) {
          setMsg(err instanceof Error ? err.message : 'ایجاد محصول ناموفق بود.')
        } finally {
          setBusy(false)
        }
      }}
      style={{ display: 'grid', gap: 'var(--kh-space-2)', maxWidth: '520px', margin: 'var(--kh-space-3) 0' }}
    >
      <label>
        عنوان
        <input value={form.title} onChange={(e) => setForm({ ...form, title: e.target.value })} required style={field} />
      </label>
      <label>
        اسلاگ (اختیاری)
        <input value={form.slug} onChange={(e) => setForm({ ...form, slug: e.target.value })} style={field} placeholder="auto" />
      </label>
      <label>
        خلاصه
        <input
          value={form.short_description}
          onChange={(e) => setForm({ ...form, short_description: e.target.value })}
          style={field}
        />
      </label>
      {msg ? <p style={{ color: 'var(--kh-text-muted)' }}>{msg}</p> : null}
      <div style={{ display: 'flex', gap: 'var(--kh-space-2)' }}>
        <Button type="submit" disabled={busy}>
          ایجاد
        </Button>
        <Button type="button" variant="ghost" onClick={() => setOpen(false)}>
          انصراف
        </Button>
      </div>
    </form>
  )
}
