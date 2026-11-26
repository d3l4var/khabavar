// Product detail: edit fields, variants, and the pricing chain (cost → price → approve).
//
// The publish gate is enforced server-side (MA-1/MA-6), so this screen never decides
// whether a product may publish: it shows the gate state the API reports and lets the
// operator fill the missing fields.
import { useState } from 'react'
import { Badge, Button, Card, Container, StateBlock, useAsync } from '../../shared/components'
import {
  adminApprovePriceRecord,
  adminCreateCostRecord,
  adminCreatePriceRecord,
  adminCreateVariant,
  adminGetProduct,
  adminListPriceRecords,
  adminPatchProduct,
  adminPublishProduct,
  formatToman,
  type AdminVariant,
} from '../../shared/api'

const field: React.CSSProperties = {
  width: '100%',
  padding: 'var(--kh-space-2)',
  borderRadius: 'var(--kh-radius-sm)',
  border: '1px solid var(--kh-border)',
  background: 'var(--kh-surface)',
  color: 'var(--kh-text)',
  font: 'inherit',
}

function VariantPanel({ v }: { v: AdminVariant }) {
  const prices = useAsync(() => adminListPriceRecords(v.id), [v.id])
  const [cost, setCost] = useState({ purchase_cost: '', supply_type: 'OWNED_INVENTORY' })
  const [price, setPrice] = useState('')
  const [msg, setMsg] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  const run = async (fn: () => Promise<unknown>, ok: string) => {
    setBusy(true)
    setMsg(null)
    try {
      await fn()
      setMsg(ok)
      prices.retry()
    } catch (err) {
      setMsg(err instanceof Error ? err.message : 'عملیات ناموفق بود.')
    } finally {
      setBusy(false)
    }
  }

  return (
    <Card>
      <div style={{ display: 'flex', gap: 'var(--kh-space-3)', alignItems: 'center', flexWrap: 'wrap' }}>
        <strong>{v.sku}</strong>
        <Badge tone="neutral">{v.status}</Badge>
      </div>

      <div style={{ display: 'grid', gap: 'var(--kh-space-2)', marginTop: 'var(--kh-space-3)', maxWidth: '420px' }}>
        <div style={{ display: 'flex', gap: 'var(--kh-space-2)' }}>
          <input
            placeholder="بهای تمام‌شده (ریال)"
            value={cost.purchase_cost}
            onChange={(e) => setCost({ ...cost, purchase_cost: e.target.value })}
            style={field}
            inputMode="numeric"
          />
          <Button
            variant="ghost"
            disabled={busy || !cost.purchase_cost}
            onClick={() =>
              run(
                () =>
                  adminCreateCostRecord(v.id, {
                    supply_type: cost.supply_type,
                    purchase_cost: Number(cost.purchase_cost),
                  }),
                'بهای تمام‌شده ثبت شد.',
              )
            }
          >
            ثبت هزینه
          </Button>
        </div>
        <div style={{ display: 'flex', gap: 'var(--kh-space-2)' }}>
          <input
            placeholder="قیمت فروش (ریال)"
            value={price}
            onChange={(e) => setPrice(e.target.value)}
            style={field}
            inputMode="numeric"
          />
          <Button
            variant="ghost"
            disabled={busy || !price}
            onClick={() => run(() => adminCreatePriceRecord(v.id, { selling_price: Number(price) }), 'قیمت پیشنهادی ثبت شد.')}
          >
            ثبت قیمت
          </Button>
        </div>
      </div>

      {msg ? <p style={{ color: 'var(--kh-text-muted)', fontSize: 'var(--kh-text-sm)' }}>{msg}</p> : null}

      {prices.loading ? <StateBlock kind="loading" /> : null}
      {prices.error ? <StateBlock kind="error" message={prices.error} onRetry={prices.retry} /> : null}
      {prices.data && (prices.data.data ?? []).length === 0 ? (
        <p style={{ color: 'var(--kh-text-muted)', fontSize: 'var(--kh-text-sm)' }}>قیمتی ثبت نشده — این گزینه قابل فروش نیست.</p>
      ) : null}
      {prices.data && (prices.data.data ?? []).length > 0 ? (
        <ul style={{ listStyle: 'none', padding: 0, display: 'grid', gap: 'var(--kh-space-2)', marginTop: 'var(--kh-space-3)' }}>
          {(prices.data.data ?? []).map((p) => (
            <li key={p.id} style={{ display: 'flex', gap: 'var(--kh-space-3)', alignItems: 'center', flexWrap: 'wrap' }}>
              <span>{formatToman(p.selling_price)}</span>
              <Badge tone={p.approved_by ? 'success' : 'warning'}>{p.approved_by ? 'APPROVED' : 'PENDING_APPROVAL'}</Badge>
              {!p.approved_by ? (
                <Button
                  variant="ghost"
                  disabled={busy}
                  onClick={() => run(() => adminApprovePriceRecord(p.id), 'قیمت تأیید شد.')}
                >
                  تأیید قیمت
                </Button>
              ) : null}
            </li>
          ))}
        </ul>
      ) : null}
    </Card>
  )
}

export function ProductDetailScreen({ id }: { id: string }) {
  const product = useAsync(() => adminGetProduct(id), [id])
  const [form, setForm] = useState<Record<string, string>>({})
  const [sku, setSku] = useState('')
  const [msg, setMsg] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  if (product.loading) return <Container><StateBlock kind="loading" /></Container>
  if (product.error) return <Container><StateBlock kind="error" message={product.error} onRetry={product.retry} /></Container>
  if (!product.data) return <Container><StateBlock kind="empty" message="محصول پیدا نشد." /></Container>

  const p = product.data
  const value = (k: string, fallback?: string) => (form[k] !== undefined ? form[k] : fallback ?? '')

  const save = async (e: React.FormEvent) => {
    e.preventDefault()
    setBusy(true)
    setMsg(null)
    try {
      const patch: Record<string, unknown> = {}
      for (const [k, v] of Object.entries(form)) if (v !== '') patch[k] = v
      if (Object.keys(patch).length === 0) {
        setMsg('تغییری وارد نشده است.')
        return
      }
      await adminPatchProduct(id, patch)
      setMsg('ذخیره شد.')
      setForm({})
      product.retry()
    } catch (err) {
      setMsg(err instanceof Error ? err.message : 'ذخیره ناموفق بود.')
    } finally {
      setBusy(false)
    }
  }

  return (
    <Container>
      <p style={{ marginTop: 'var(--kh-space-5)' }}>
        <a href="#/products">‹ محصولات</a>
      </p>
      <h1>{p.title}</h1>
      <p style={{ display: 'flex', gap: 'var(--kh-space-3)', alignItems: 'center', flexWrap: 'wrap' }}>
        <Badge tone={p.status === 'PUBLISHED' ? 'success' : 'neutral'}>{p.status}</Badge>
        <Badge tone={p.publish_gate_satisfied ? 'success' : 'error'}>
          {p.publish_gate_satisfied ? 'دروازهٔ انتشار کامل' : 'دروازهٔ انتشار ناقص'}
        </Badge>
        <span style={{ color: 'var(--kh-text-muted)', fontSize: 'var(--kh-text-sm)' }}>{p.slug}</span>
        {p.status !== 'PUBLISHED' ? (
          <Button
            disabled={busy || !p.publish_gate_satisfied}
            onClick={async () => {
              setBusy(true)
              try {
                await adminPublishProduct(id)
                setMsg('محصول منتشر شد.')
                product.retry()
              } catch (err) {
                setMsg(err instanceof Error ? err.message : 'انتشار ناموفق بود.')
              } finally {
                setBusy(false)
              }
            }}
          >
            انتشار
          </Button>
        ) : null}
      </p>

      <h2>ویرایش</h2>
      <form onSubmit={save} style={{ display: 'grid', gap: 'var(--kh-space-2)', maxWidth: '560px' }}>
        <label>
          عنوان
          <input value={value('title', p.title)} onChange={(e) => setForm({ ...form, title: e.target.value })} style={field} />
        </label>
        <label>
          خلاصه
          <input
            value={value('short_description', p.short_description)}
            onChange={(e) => setForm({ ...form, short_description: e.target.value })}
            style={field}
          />
        </label>
        <label>
          توضیح
          <textarea
            value={value('description', p.description)}
            onChange={(e) => setForm({ ...form, description: e.target.value })}
            rows={3}
            style={field}
          />
        </label>
        <label>
          منبع مشخصات
          <input value={value('spec_source', p.spec_source)} onChange={(e) => setForm({ ...form, spec_source: e.target.value })} style={field} />
        </label>
        <label>
          ارتباط با خواب
          <select value={value('sleep_relevance', p.sleep_relevance)} onChange={(e) => setForm({ ...form, sleep_relevance: e.target.value })} style={field}>
            <option value="">—</option>
            <option value="SLEEP_RELATED">SLEEP_RELATED</option>
            <option value="SLEEP_ADJACENT">SLEEP_ADJACENT</option>
            <option value="NOT_RELATED">NOT_RELATED</option>
          </select>
        </label>
        {msg ? <p style={{ color: 'var(--kh-text-muted)' }}>{msg}</p> : null}
        <Button type="submit" disabled={busy}>
          ذخیره
        </Button>
      </form>

      <h2 style={{ marginTop: 'var(--kh-space-6)' }}>گزینه‌ها و قیمت</h2>
      <div style={{ display: 'grid', gap: 'var(--kh-space-3)' }}>
        {(p.variants ?? []).map((v) => (
          <VariantPanel key={v.id} v={v} />
        ))}
        {(p.variants ?? []).length === 0 ? (
          <StateBlock kind="empty" message="این محصول هنوز گزینه‌ای ندارد؛ بدون گزینه قابل فروش نیست." />
        ) : null}
      </div>

      <form
        onSubmit={async (e) => {
          e.preventDefault()
          setBusy(true)
          try {
            await adminCreateVariant({ product_id: id, sku: sku.trim() })
            setSku('')
            setMsg('گزینه اضافه شد.')
            product.retry()
          } catch (err) {
            setMsg(err instanceof Error ? err.message : 'افزودن گزینه ناموفق بود.')
          } finally {
            setBusy(false)
          }
        }}
        style={{ display: 'flex', gap: 'var(--kh-space-2)', marginTop: 'var(--kh-space-3)', maxWidth: '420px' }}
      >
        <input placeholder="SKU گزینهٔ جدید" value={sku} onChange={(e) => setSku(e.target.value)} style={field} required />
        <Button type="submit" disabled={busy}>
          افزودن گزینه
        </Button>
      </form>
    </Container>
  )
}
