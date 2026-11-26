// KHABAVAR — shop (shop.khabavar.ir).
//
// The cart is client-side (localStorage) because the backend exposes orders, not carts;
// checkout posts one order with the variant snapshots and the live storefront price.
// Prices are always re-read from /variants/{id}/price — never trusted from the cart —
// so a stale tab cannot submit an old price.
import { useEffect, useMemo, useState } from 'react'
import { createRoot } from 'react-dom/client'
import { Button, Card, Container, Footer, Grid, Header, StateBlock, useAsync } from '../../shared/components'
import {
  createOrder,
  formatToman,
  getOrderByNumber,
  getProduct,
  getVariantPrice,
  searchProducts,
  type OrderItemInput,
  type Product,
} from '../../shared/api'
import '../../shared/tokens.css'

const CART_KEY = 'kh.cart.v1'

type CartLine = {
  productId: string
  variantId: string
  title: string
  sku: string
  quantity: number
  unitPrice: number // IRR, snapshot for display only
}

function readCart(): CartLine[] {
  try {
    const raw = localStorage.getItem(CART_KEY)
    return raw ? (JSON.parse(raw) as CartLine[]) : []
  } catch {
    return []
  }
}

function writeCart(lines: CartLine[]) {
  localStorage.setItem(CART_KEY, JSON.stringify(lines))
}

function useCart() {
  const [lines, setLines] = useState<CartLine[]>(() => readCart())
  const update = (next: CartLine[]) => {
    setLines(next)
    writeCart(next)
  }
  return {
    lines,
    add(line: CartLine) {
      const existing = lines.find((l) => l.variantId === line.variantId)
      update(existing ? lines.map((l) => (l.variantId === line.variantId ? { ...l, quantity: l.quantity + line.quantity } : l)) : [...lines, line])
    },
    setQty(variantId: string, qty: number) {
      update(qty <= 0 ? lines.filter((l) => l.variantId !== variantId) : lines.map((l) => (l.variantId === variantId ? { ...l, quantity: qty } : l)))
    },
    clear() {
      update([])
    },
  }
}

function useHashRoute() {
  const [hash, setHash] = useState(() => window.location.hash || '#/')
  useEffect(() => {
    const onChange = () => setHash(window.location.hash || '#/')
    window.addEventListener('hashchange', onChange)
    return () => window.removeEventListener('hashchange', onChange)
  }, [])
  return hash
}

function parseRoute(hash: string) {
  const segs = hash.replace(/^#\/?/, '').split('/').filter(Boolean)
  if (segs.length === 0) return { name: 'list', param: '' }
  if (segs[0] === 'product' && segs[1]) return { name: 'product', param: decodeURIComponent(segs[1]) }
  if (segs[0] === 'order' && segs[1]) return { name: 'order', param: decodeURIComponent(segs[1]) }
  return { name: segs[0], param: segs[1] ?? '' }
}

// ── screens ───────────────────────────────────────────────────────────────────

function ListScreen({ cart }: { cart: ReturnType<typeof useCart> }) {
  const [term, setTerm] = useState('')
  const [submitted, setSubmitted] = useState('')
  const result = useAsync(() => searchProducts(submitted, 1, 24), [submitted])

  return (
    <Container>
      <h1 style={{ marginTop: 'var(--kh-space-5)' }}>فروشگاه خواب‌آور</h1>
      <form
        onSubmit={(e) => {
          e.preventDefault()
          setSubmitted(term.trim())
        }}
        style={{ display: 'flex', gap: 'var(--kh-space-2)', margin: 'var(--kh-space-4) 0' }}
      >
        <input
          value={term}
          onChange={(e) => setTerm(e.target.value)}
          placeholder="جست‌وجوی محصول…"
          aria-label="جست‌وجوی محصول"
          style={{ flex: 1, padding: 'var(--kh-space-3)', borderRadius: 'var(--kh-radius-sm)', border: '1px solid var(--kh-border)', background: 'var(--kh-surface)', color: 'var(--kh-text)', font: 'inherit' }}
        />
        <Button type="submit">جست‌وجو</Button>
        <Button href="#/cart" variant="ghost">
          سبد ({cart.lines.length})
        </Button>
      </form>

      {result.loading ? <StateBlock kind="loading" /> : null}
      {result.error ? <StateBlock kind="error" message={result.error} onRetry={result.retry} /> : null}
      {result.data && (result.data.data ?? []).length === 0 ? <StateBlock kind="empty" message="محصولی پیدا نشد." /> : null}
      {result.data && (result.data.data ?? []).length > 0 ? (
        <Grid>
          {(result.data.data ?? []).map((hit) => {
            const p: Product = hit.Product
            return (
              <Card key={p.id}>
                <h3 style={{ margin: 0 }}>
                  <a href={`#/product/${encodeURIComponent(p.id)}`} style={{ color: 'var(--kh-text)' }}>
                    {p.title}
                  </a>
                </h3>
                {p.short_description ? <p style={{ margin: 0, color: 'var(--kh-text-muted)', fontSize: 'var(--kh-text-sm)' }}>{p.short_description}</p> : null}
              </Card>
            )
          })}
        </Grid>
      ) : null}
    </Container>
  )
}

function ProductScreen({ id, cart }: { id: string; cart: ReturnType<typeof useCart> }) {
  const product = useAsync(() => getProduct(id), [id])
  const [picked, setPicked] = useState<string>('')
  const variantId = picked || product.data?.data.variants?.[0]?.id || ''
  const price = useAsync(async () => (variantId ? getVariantPrice(variantId) : null), [variantId])

  if (product.loading) return <Container><StateBlock kind="loading" /></Container>
  if (product.error) return <Container><StateBlock kind="error" message={product.error} onRetry={product.retry} /></Container>
  if (!product.data) return <Container><StateBlock kind="empty" /></Container>

  const p = product.data.data
  const variants = p.variants ?? []
  const variant = variants.find((v) => v.id === variantId)

  return (
    <Container>
      <p style={{ marginTop: 'var(--kh-space-5)' }}>
        <a href="#/">‹ بازگشت به فروشگاه</a> · <a href="#/cart">سبد ({cart.lines.length})</a>
      </p>
      <h1>{p.title}</h1>
      {p.description ? <p>{p.description}</p> : null}

      {variants.length === 0 ? (
        <StateBlock kind="empty" message="برای این محصول هنوز گزینه‌ای ثبت نشده است." />
      ) : (
        <>
          <label htmlFor="variant" style={{ display: 'block', marginBottom: 'var(--kh-space-2)' }}>
            گزینه
          </label>
          <select
            id="variant"
            value={variantId}
            onChange={(e) => setPicked(e.target.value)}
            style={{ padding: 'var(--kh-space-2)', borderRadius: 'var(--kh-radius-sm)', border: '1px solid var(--kh-border)', background: 'var(--kh-surface)', color: 'var(--kh-text)', font: 'inherit' }}
          >
            {variants.map((v) => (
              <option key={v.id} value={v.id}>
                {v.sku}
              </option>
            ))}
          </select>

          <div style={{ margin: 'var(--kh-space-4) 0' }}>
            {price.loading ? <StateBlock kind="loading" /> : null}
            {price.error ? <StateBlock kind="empty" message="این گزینه فعلاً قیمت مصوب ندارد و قابل فروش نیست." /> : null}
            {price.data?.data ? (
              <p style={{ fontSize: 'var(--kh-text-xl)', margin: 0 }}>{formatToman(price.data.data.selling_price)}</p>
            ) : null}
          </div>

          <Button
            disabled={!price.data?.data || !variant}
            onClick={() => {
              if (!price.data?.data || !variant) return
              cart.add({
                productId: p.id,
                variantId: variant.id,
                title: p.title,
                sku: variant.sku,
                quantity: 1,
                unitPrice: price.data.data.selling_price,
              })
              window.location.hash = '#/cart'
            }}
          >
            افزودن به سبد
          </Button>
        </>
      )}
    </Container>
  )
}

function CartScreen({ cart }: { cart: ReturnType<typeof useCart> }) {
  const total = useMemo(() => cart.lines.reduce((sum, l) => sum + l.unitPrice * l.quantity, 0), [cart.lines])
  return (
    <Container>
      <h1 style={{ marginTop: 'var(--kh-space-5)' }}>سبد خرید</h1>
      {cart.lines.length === 0 ? <StateBlock kind="empty" message="سبد خالی است." /> : null}
      {cart.lines.length > 0 ? (
        <>
          <ul style={{ listStyle: 'none', padding: 0, display: 'grid', gap: 'var(--kh-space-3)' }}>
            {cart.lines.map((l) => (
              <li key={l.variantId}>
                <Card>
                  <strong>{l.title}</strong>
                  <span style={{ color: 'var(--kh-text-muted)', fontSize: 'var(--kh-text-sm)' }}> کد {l.sku}</span>
                  <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--kh-space-2)' }}>
                    <button onClick={() => cart.setQty(l.variantId, l.quantity - 1)} aria-label="کاهش">−</button>
                    <span>{l.quantity}</span>
                    <button onClick={() => cart.setQty(l.variantId, l.quantity + 1)} aria-label="افزایش">+</button>
                    <span style={{ marginInlineStart: 'auto' }}>{formatToman(l.unitPrice * l.quantity)}</span>
                  </div>
                </Card>
              </li>
            ))}
          </ul>
          <p style={{ fontSize: 'var(--kh-text-lg)' }}>جمع: {formatToman(total)}</p>
          <Button href="#/checkout">ادامهٔ تسویه</Button>
        </>
      ) : null}
    </Container>
  )
}

function CheckoutScreen({ cart }: { cart: ReturnType<typeof useCart> }) {
  const [phone, setPhone] = useState('')
  const [city, setCity] = useState('')
  const [province, setProvince] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError(null)
    if (cart.lines.length === 0) {
      setError('سبد خالی است.')
      return
    }
    setBusy(true)
    try {
      // Re-read the live price for every line: never submit a stale cart price.
      const items: OrderItemInput[] = []
      for (const line of cart.lines) {
        const live = await getVariantPrice(line.variantId)
        items.push({
          product_id: line.productId,
          variant_id: line.variantId,
          sku_snapshot: line.sku,
          title_snapshot: line.title,
          quantity: line.quantity,
          unit_price: live.data.selling_price,
          discount_amount: 0,
          tax_amount: 0,
          supply_type: 'OWNED_INVENTORY',
        })
      }
      const created = await createOrder({
        guest_phone: phone,
        items,
        shipping_address: { city, province },
        discount_total: 0,
        shipping_total: 0,
        tax_total: 0,
      })
      cart.clear()
      window.location.hash = `#/order/${encodeURIComponent(created.id)}`
    } catch (err) {
      setError(err instanceof Error ? err.message : 'ثبت سفارش ناموفق بود.')
    } finally {
      setBusy(false)
    }
  }

  return (
    <Container>
      <h1 style={{ marginTop: 'var(--kh-space-5)' }}>تسویه</h1>
      <form onSubmit={submit} style={{ display: 'grid', gap: 'var(--kh-space-3)', maxWidth: '420px' }}>
        <label>
          شمارهٔ همراه
          <input value={phone} onChange={(e) => setPhone(e.target.value)} required placeholder="09xxxxxxxxx" style={{ width: '100%', padding: 'var(--kh-space-3)', marginTop: 'var(--kh-space-1)' }} />
        </label>
        <label>
          استان
          <input value={province} onChange={(e) => setProvince(e.target.value)} required style={{ width: '100%', padding: 'var(--kh-space-3)', marginTop: 'var(--kh-space-1)' }} />
        </label>
        <label>
          شهر
          <input value={city} onChange={(e) => setCity(e.target.value)} required style={{ width: '100%', padding: 'var(--kh-space-3)', marginTop: 'var(--kh-space-1)' }} />
        </label>
        {error ? <StateBlock kind="error" message={error} /> : null}
        <Button type="submit" disabled={busy}>
          {busy ? 'در حال ثبت…' : 'ثبت سفارش'}
        </Button>
      </form>
      <p style={{ color: 'var(--kh-text-muted)', fontSize: 'var(--kh-text-sm)' }}>
        پرداخت در این نسخه Mock است؛ سفارش با وضعیت «در انتظار پرداخت» ثبت می‌شود.
      </p>
    </Container>
  )
}

function OrderScreen({ id }: { id: string }) {
  const order = useAsync(() => getOrderByNumber(id), [id])
  if (order.loading) return <Container><StateBlock kind="loading" /></Container>
  if (order.error) return <Container><StateBlock kind="error" message={order.error} onRetry={order.retry} /></Container>
  if (!order.data) return <Container><StateBlock kind="empty" /></Container>
  const o = order.data
  return (
    <Container>
      <h1 style={{ marginTop: 'var(--kh-space-5)' }}>سفارش ثبت شد</h1>
      <Card>
        <p style={{ margin: 0 }}>شمارهٔ سفارش: <strong>{o.order_number}</strong></p>
        <p style={{ margin: 0 }}>وضعیت: {o.status}</p>
        <p style={{ margin: 0 }}>مبلغ: {formatToman(o.grand_total)}</p>
      </Card>
      <p style={{ marginTop: 'var(--kh-space-4)' }}>
        <a href="#/">بازگشت به فروشگاه</a>
      </p>
    </Container>
  )
}

const NAV = [
  { label: 'فروشگاه', href: '#/' },
  { label: 'سبد', href: '#/cart' },
  { label: 'خانه', href: 'https://khabavar.ir' },
  { label: 'مجله', href: 'https://magazine.khabavar.ir' },
]

function App() {
  const hash = useHashRoute()
  const route = useMemo(() => parseRoute(hash), [hash])
  const cart = useCart()

  let screen: JSX.Element
  switch (route.name) {
    case 'list':
      screen = <ListScreen cart={cart} />
      break
    case 'product':
      screen = <ProductScreen id={route.param} cart={cart} />
      break
    case 'cart':
      screen = <CartScreen cart={cart} />
      break
    case 'checkout':
      screen = <CheckoutScreen cart={cart} />
      break
    case 'order':
      screen = <OrderScreen id={route.param} />
      break
    default:
      screen = <Container><StateBlock kind="empty" message="صفحه پیدا نشد." /></Container>
  }

  return (
    <div style={{ minHeight: '100%', display: 'flex', flexDirection: 'column' }}>
      <Header items={NAV} />
      <main style={{ flex: 1 }}>{screen}</main>
      <Footer />
    </div>
  )
}

createRoot(document.getElementById('root')!).render(<App />)
