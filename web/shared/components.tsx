// KHABAVAR — shared UI primitives (P2 design system).
//
// Small, dependency-free components used by all four apps. They consume design tokens
// only (no hard-coded colours) and use logical CSS properties so RTL is native.
import { useEffect, useState, type ReactNode } from 'react'

export function Container({ children, className }: { children: ReactNode; className?: string }) {
  return (
    <div className={className} style={{ maxWidth: 'var(--kh-container)', margin: '0 auto', padding: '0 var(--kh-space-4)' }}>
      {children}
    </div>
  )
}

export function Button({
  children,
  onClick,
  href,
  variant = 'primary',
  type = 'button',
  disabled,
}: {
  children: ReactNode
  onClick?: () => void
  href?: string
  variant?: 'primary' | 'ghost'
  type?: 'button' | 'submit'
  disabled?: boolean
}) {
  const style: React.CSSProperties = {
    display: 'inline-flex',
    alignItems: 'center',
    gap: 'var(--kh-space-2)',
    padding: 'var(--kh-space-3) var(--kh-space-5)',
    borderRadius: 'var(--kh-radius-sm)',
    border: variant === 'primary' ? '1px solid transparent' : '1px solid var(--kh-border)',
    background: variant === 'primary' ? 'var(--kh-accent-strong)' : 'transparent',
    color: variant === 'primary' ? 'var(--kh-on-accent)' : 'var(--kh-text)',
    font: 'inherit',
    cursor: disabled ? 'not-allowed' : 'pointer',
    opacity: disabled ? 0.6 : 1,
  }
  if (href) {
    return (
      <a href={href} style={style}>
        {children}
      </a>
    )
  }
  return (
    <button type={type} onClick={onClick} disabled={disabled} style={style}>
      {children}
    </button>
  )
}

export function Badge({ children, tone = 'neutral' }: { children: ReactNode; tone?: 'neutral' | 'success' | 'warning' | 'error' }) {
  const color =
    tone === 'success' ? 'var(--kh-success)' : tone === 'warning' ? 'var(--kh-warning)' : tone === 'error' ? 'var(--kh-error)' : 'var(--kh-accent)'
  return (
    <span
      style={{
        display: 'inline-block',
        padding: '2px var(--kh-space-2)',
        borderRadius: '999px',
        border: `1px solid ${color}`,
        color,
        fontSize: 'var(--kh-text-sm)',
      }}
    >
      {children}
    </span>
  )
}

export function Card({ children }: { children: ReactNode }) {
  return (
    <article
      style={{
        background: 'var(--kh-surface)',
        border: '1px solid var(--kh-border)',
        borderRadius: 'var(--kh-radius)',
        padding: 'var(--kh-space-4)',
        boxShadow: 'var(--kh-shadow)',
        display: 'flex',
        flexDirection: 'column',
        gap: 'var(--kh-space-2)',
      }}
    >
      {children}
    </article>
  )
}

// StateBlock renders the three shared states the design system requires
// (Loading / Empty / Error) so every screen behaves the same way.
export function StateBlock({ kind, message, onRetry }: { kind: 'loading' | 'empty' | 'error'; message?: string; onRetry?: () => void }) {
  const text =
    kind === 'loading' ? 'در حال بارگذاری…' : kind === 'empty' ? (message ?? 'موردی برای نمایش نیست.') : (message ?? 'خطایی رخ داد.')
  const color = kind === 'error' ? 'var(--kh-error)' : 'var(--kh-text-muted)'
  return (
    <div role={kind === 'error' ? 'alert' : 'status'} style={{ padding: 'var(--kh-space-6)', textAlign: 'center', color }}>
      <p style={{ margin: 0 }}>{text}</p>
      {kind === 'error' && onRetry ? (
        <div style={{ marginTop: 'var(--kh-space-3)' }}>
          <Button variant="ghost" onClick={onRetry}>
            تلاش دوباره
          </Button>
        </div>
      ) : null}
    </div>
  )
}

// useAsync is the one loading primitive: it tracks loading/error/data and exposes a
// retry, so no screen has to re-implement the three states.
// humanError turns any thrown value into a sentence a Persian-speaking customer can read.
// The API's own errors already carry a code, so they are shown as-is; everything else is a
// transport failure and says so.
function humanError(err: unknown): string {
  if (err && typeof err === 'object' && 'code' in err && typeof (err as { code?: unknown }).code === 'string') {
    return err instanceof Error ? err.message : 'خطا در دریافت داده.'
  }
  return 'ارتباط با سرور برقرار نشد. اتصال اینترنت را بررسی کنید و دوباره تلاش کنید.'
}

export function useAsync<T>(fn: () => Promise<T>, deps: unknown[] = []) {
  const [state, setState] = useState<{ loading: boolean; error: string | null; data: T | null }>({
    loading: true,
    error: null,
    data: null,
  })
  const [tick, setTick] = useState(0)

  useEffect(() => {
    let alive = true
    setState((s) => ({ ...s, loading: true, error: null }))
    fn()
      .then((data) => alive && setState({ loading: false, error: null, data }))
      // A network failure surfaces as a TypeError with an English message ("fetch error",
      // "Failed to fetch"). A Persian UI must not show that, so anything that is not an API
      // error becomes one sentence the customer can act on.
      .catch((err: unknown) => alive && setState({ loading: false, error: humanError(err), data: null }))
    return () => {
      alive = false
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [...deps, tick])

  return { ...state, retry: () => setTick((t) => t + 1) }
}

// ── layout chrome ──

export function Header({ items }: { items: { label: string; href: string }[] }) {
  return (
    <header
      className="kh-dark"
      style={{
        position: 'sticky',
        top: 0,
        zIndex: 10,
        background: 'var(--kh-bg)',
        borderBottom: '1px solid var(--kh-border)',
        height: 'var(--kh-header-h)',
        display: 'flex',
        alignItems: 'center',
      }}
    >
      <Container>
        <nav style={{ display: 'flex', alignItems: 'center', gap: 'var(--kh-space-5)', height: 'var(--kh-header-h)' }}>
          <a href="#/" style={{ fontWeight: 700, color: 'var(--kh-text)', fontSize: 'var(--kh-text-lg)' }}>
            خواب‌آور
          </a>
          <ul style={{ display: 'flex', gap: 'var(--kh-space-4)', listStyle: 'none', margin: 0, padding: 0, flexWrap: 'wrap' }}>
            {items.map((it) => (
              <li key={it.href}>
                <a href={it.href} style={{ color: 'var(--kh-text-muted)' }}>
                  {it.label}
                </a>
              </li>
            ))}
          </ul>
        </nav>
      </Container>
    </header>
  )
}

export function Footer() {
  return (
    <footer className="kh-dark" style={{ background: 'var(--kh-bg)', borderTop: '1px solid var(--kh-border)', marginTop: 'var(--kh-space-7)' }}>
      <Container>
        <div style={{ padding: 'var(--kh-space-6) 0', color: 'var(--kh-text-muted)', fontSize: 'var(--kh-text-sm)' }}>
          <p style={{ margin: 0 }}>خواب‌آور — اکوسیستم تجارت و محتوای تخصصی خواب</p>
          <p style={{ margin: 'var(--kh-space-2) 0 0' }}>محتوای مجله جایگزین توصیه پزشکی نیست.</p>
        </div>
      </Container>
    </footer>
  )
}

// ── domain cards ──

export function ProductCard({ product, href }: { product: { id: string; title: string; short_description?: string; sleep_relevance?: string }; href: string }) {
  return (
    <Card>
      <h3 style={{ margin: 0 }}>
        <a href={href} style={{ color: 'var(--kh-text)' }}>
          {product.title}
        </a>
      </h3>
      {product.short_description ? (
        <p style={{ margin: 0, color: 'var(--kh-text-muted)', fontSize: 'var(--kh-text-sm)' }}>{product.short_description}</p>
      ) : null}
      {product.sleep_relevance ? <Badge tone="success">{product.sleep_relevance}</Badge> : null}
    </Card>
  )
}

export function ArticleCard({ article, href }: { article: { title: string; summary?: string }; href: string }) {
  return (
    <Card>
      <h3 style={{ margin: 0 }}>
        <a href={href} style={{ color: 'var(--kh-text)' }}>
          {article.title}
        </a>
      </h3>
      {article.summary ? (
        <p style={{ margin: 0, color: 'var(--kh-text-muted)', fontSize: 'var(--kh-text-sm)' }}>{article.summary}</p>
      ) : null}
    </Card>
  )
}

// Grid is the one layout primitive: responsive without media-query duplication.
export function Grid({ children, min = '260px' }: { children: ReactNode; min?: string }) {
  return (
    <div style={{ display: 'grid', gap: 'var(--kh-space-4)', gridTemplateColumns: `repeat(auto-fill, minmax(${min}, 1fr))` }}>
      {children}
    </div>
  )
}
