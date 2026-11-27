// KHABAVAR — main site (khabavar.ir).
//
// Routes (hash-based, no router dependency — the dep list is deliberately tiny):
//   #/              home: latest articles + featured products
//   #/shop          product search / listing
//   #/product/:id   product detail
//   #/magazine      article listing
//   #/article/:slug article detail
//   #/search?q=     unified search across products and articles
import { useEffect, useMemo, useState } from 'react'
import { createRoot } from 'react-dom/client'
import {
  ArticleCard,
  Button,
  Container,
  Footer,
  Grid,
  Header,
  ProductCard,
  StateBlock,
  useAsync,
} from '../../shared/components'
import {
  getArticle,
  getProduct,
  listArticles,
  listTopics,
  navigation,
  searchProducts,
  unifiedSearch,
  type Article,
  type Product,
} from '../../shared/api'
import { ErrorBoundary } from '../../shared/ErrorBoundary'
import '../../shared/tokens.css'

// ── tiny hash router ──────────────────────────────────────────────────────────
function useHashRoute() {
  const [hash, setHash] = useState(() => window.location.hash || '#/')
  useEffect(() => {
    const onChange = () => setHash(window.location.hash || '#/')
    window.addEventListener('hashchange', onChange)
    return () => window.removeEventListener('hashchange', onChange)
  }, [])
  return hash
}

function parseRoute(hash: string): { name: string; param: string; query: URLSearchParams } {
  const raw = hash.replace(/^#\/?/, '')
  const [pathPart, queryPart] = raw.split('?')
  const segments = pathPart.split('/').filter(Boolean)
  const query = new URLSearchParams(queryPart ?? '')
  if (segments.length === 0) return { name: 'home', param: '', query }
  if (segments[0] === 'product' && segments[1]) return { name: 'product', param: decodeURIComponent(segments[1]), query }
  if (segments[0] === 'article' && segments[1]) return { name: 'article', param: decodeURIComponent(segments[1]), query }
  return { name: segments[0], param: segments[1] ?? '', query }
}

// ── screens ───────────────────────────────────────────────────────────────────

function HomeScreen() {
  const articles = useAsync(() => listArticles(1, 6), [])
  const products = useAsync(() => searchProducts('', 1, 6), [])

  return (
    <Container>
      <section className="kh-dark" style={{ background: 'var(--kh-bg)', borderRadius: 'var(--kh-radius-lg)', padding: 'var(--kh-space-7) var(--kh-space-5)', margin: 'var(--kh-space-5) 0' }}>
        <h1>خواب بهتر، از آگاهی شروع می‌شود</h1>
        <p style={{ color: 'var(--kh-text-muted)', maxWidth: '46ch', margin: '0 0 var(--kh-space-4)' }}>
          مجله تخصصی خواب و فروشگاه محصولات خواب‌آور — با منبع، بدون ادعای درمان.
        </p>
        <div style={{ display: 'flex', gap: 'var(--kh-space-3)', flexWrap: 'wrap' }}>
          <Button href="#/shop">ورود به فروشگاه</Button>
          <Button href="#/magazine" variant="ghost">
            مطالعه مجله
          </Button>
        </div>
      </section>

      <section style={{ margin: 'var(--kh-space-6) 0' }}>
        <h2>تازه‌ترین مقالات</h2>
        {articles.loading ? <StateBlock kind="loading" /> : null}
        {articles.error ? <StateBlock kind="error" message={articles.error} onRetry={articles.retry} /> : null}
        {articles.data && (articles.data.data ?? []).length === 0 ? <StateBlock kind="empty" message="هنوز مقاله‌ای منتشر نشده است." /> : null}
        {articles.data && (articles.data.data ?? []).length > 0 ? (
          <Grid>
            {(articles.data.data ?? []).map((a: Article) => (
              <ArticleCard key={a.id} article={a} href={`#/article/${encodeURIComponent(a.slug)}`} />
            ))}
          </Grid>
        ) : null}
      </section>

      <section style={{ margin: 'var(--kh-space-6) 0' }}>
        <h2>محصولات</h2>
        {products.loading ? <StateBlock kind="loading" /> : null}
        {products.error ? <StateBlock kind="error" message={products.error} onRetry={products.retry} /> : null}
        {products.data && (products.data.data ?? []).length === 0 ? <StateBlock kind="empty" message="محصولی برای نمایش نیست." /> : null}
        {products.data && (products.data.data ?? []).length > 0 ? (
          <Grid>
            {(products.data.data ?? []).map((hit) => (
              <ProductCard key={hit.product.id} product={hit.product} href={`#/product/${encodeURIComponent(hit.product.id)}`} />
            ))}
          </Grid>
        ) : null}
      </section>
    </Container>
  )
}

function ShopScreen() {
  const [term, setTerm] = useState('')
  const [submitted, setSubmitted] = useState('')
  const result = useAsync(() => searchProducts(submitted, 1, 24), [submitted])

  return (
    <Container>
      <h1 style={{ marginTop: 'var(--kh-space-5)' }}>فروشگاه</h1>
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
          style={{
            flex: 1,
            padding: 'var(--kh-space-3)',
            borderRadius: 'var(--kh-radius-sm)',
            border: '1px solid var(--kh-border)',
            background: 'var(--kh-surface)',
            color: 'var(--kh-text)',
            font: 'inherit',
          }}
        />
        <Button type="submit">جست‌وجو</Button>
      </form>

      {result.loading ? <StateBlock kind="loading" /> : null}
      {result.error ? <StateBlock kind="error" message={result.error} onRetry={result.retry} /> : null}
      {result.data && (result.data.data ?? []).length === 0 ? <StateBlock kind="empty" message="محصولی با این مشخصات پیدا نشد." /> : null}
      {result.data && (result.data.data ?? []).length > 0 ? (
        <Grid>
          {(result.data.data ?? []).map((hit) => (
            <ProductCard key={hit.product.id} product={hit.product} href={`#/product/${encodeURIComponent(hit.product.id)}`} />
          ))}
        </Grid>
      ) : null}
    </Container>
  )
}

function ProductScreen({ id }: { id: string }) {
  const product = useAsync(() => getProduct(id), [id])
  if (product.loading) return <Container><StateBlock kind="loading" /></Container>
  if (product.error) return <Container><StateBlock kind="error" message={product.error} onRetry={product.retry} /></Container>
  if (!product.data) return <Container><StateBlock kind="empty" /></Container>

  const p: Product = product.data.data
  return (
    <Container>
      <p style={{ marginTop: 'var(--kh-space-5)' }}>
        <a href="#/shop">‹ بازگشت به فروشگاه</a>
      </p>
      <h1>{p.title}</h1>
      {p.sleep_relevance ? <p style={{ color: 'var(--kh-text-muted)' }}>ارتباط با خواب: {p.sleep_relevance}</p> : null}
      {p.description ? <p>{p.description}</p> : null}

      <h2>گزینه‌ها</h2>
      {p.variants && p.variants.length > 0 ? (
        <ul style={{ paddingInlineStart: 'var(--kh-space-5)' }}>
          {p.variants.map((v) => (
            <li key={v.id}>کد کالا: {v.sku}</li>
          ))}
        </ul>
      ) : (
        <StateBlock kind="empty" message="برای این محصول هنوز گزینه‌ای ثبت نشده است." />
      )}
      {/* Price is loaded per variant from /variants/{id}/price; the storefront hides the
          confidential cost fields (R-05 §۲۴). */}
      <p style={{ color: 'var(--kh-text-muted)', fontSize: 'var(--kh-text-sm)' }}>
        قیمت هر گزینه پس از انتخاب نمایش داده می‌شود.
      </p>
    </Container>
  )
}

function MagazineScreen() {
  const articles = useAsync(() => listArticles(1, 24), [])
  const topics = useAsync(() => listTopics(), [])
  return (
    <Container>
      <h1 style={{ marginTop: 'var(--kh-space-5)' }}>مجله</h1>
      {topics.data && (topics.data.data ?? []).length > 0 ? (
        <div style={{ display: 'flex', gap: 'var(--kh-space-2)', flexWrap: 'wrap', margin: 'var(--kh-space-3) 0' }}>
          {(topics.data.data ?? []).map((t) => (
            <span key={t.id} style={{ fontSize: 'var(--kh-text-sm)', color: 'var(--kh-text-muted)' }}>
              {t.name}
            </span>
          ))}
        </div>
      ) : null}

      {articles.loading ? <StateBlock kind="loading" /> : null}
      {articles.error ? <StateBlock kind="error" message={articles.error} onRetry={articles.retry} /> : null}
      {articles.data && (articles.data.data ?? []).length === 0 ? <StateBlock kind="empty" message="هنوز مقاله‌ای منتشر نشده است." /> : null}
      {articles.data && (articles.data.data ?? []).length > 0 ? (
        <Grid>
          {(articles.data.data ?? []).map((a) => (
            <ArticleCard key={a.id} article={a} href={`#/article/${encodeURIComponent(a.slug)}`} />
          ))}
        </Grid>
      ) : null}
    </Container>
  )
}

function ArticleScreen({ slug }: { slug: string }) {
  const article = useAsync(() => getArticle(slug), [slug])
  if (article.loading) return <Container><StateBlock kind="loading" /></Container>
  if (article.error) return <Container><StateBlock kind="error" message={article.error} onRetry={article.retry} /></Container>
  if (!article.data) return <Container><StateBlock kind="empty" /></Container>

  const a = article.data.data
  return (
    <Container>
      <p style={{ marginTop: 'var(--kh-space-5)' }}>
        <a href="#/magazine">‹ بازگشت به مجله</a>
      </p>
      <article style={{ maxWidth: '68ch' }}>
        <h1>{a.title}</h1>
        {a.published_at ? (
          <p style={{ color: 'var(--kh-text-muted)', fontSize: 'var(--kh-text-sm)' }}>
            {new Date(a.published_at).toLocaleDateString('fa-IR')}
          </p>
        ) : null}
        {a.summary ? <p style={{ fontSize: 'var(--kh-text-lg)' }}>{a.summary}</p> : null}
        <div style={{ whiteSpace: 'pre-wrap' }}>{a.body}</div>
      </article>
    </Container>
  )
}

function SearchScreen({ initial }: { initial: string }) {
  const [term, setTerm] = useState(initial)
  const [submitted, setSubmitted] = useState(initial)
  const result = useAsync(() => unifiedSearch(submitted, 1, 12), [submitted])

  return (
    <Container>
      <h1 style={{ marginTop: 'var(--kh-space-5)' }}>جست‌وجو</h1>
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
          placeholder="در محصولات و مقالات…"
          aria-label="جست‌وجوی یکپارچه"
          style={{ flex: 1, padding: 'var(--kh-space-3)', borderRadius: 'var(--kh-radius-sm)', border: '1px solid var(--kh-border)', background: 'var(--kh-surface)', color: 'var(--kh-text)', font: 'inherit' }}
        />
        <Button type="submit">جست‌وجو</Button>
      </form>

      {result.loading ? <StateBlock kind="loading" /> : null}
      {result.error ? <StateBlock kind="error" message={result.error} onRetry={result.retry} /> : null}
      {result.data && result.data.items.length === 0 ? <StateBlock kind="empty" message="نتیجه‌ای پیدا نشد." /> : null}
      {result.data && result.data.items.length > 0 ? (
        <>
          <p style={{ color: 'var(--kh-text-muted)', fontSize: 'var(--kh-text-sm)' }}>
            {result.data.product_total} محصول و {result.data.article_total} مقاله
          </p>
          <ul style={{ listStyle: 'none', padding: 0, display: 'grid', gap: 'var(--kh-space-3)' }}>
            {result.data.items.map((hit) => (
              <li key={`${hit.type}-${hit.id}`}>
                <a href={hit.type === 'product' ? `#/product/${encodeURIComponent(hit.id)}` : `#/article/${encodeURIComponent(hit.slug)}`}>
                  {hit.title}
                </a>{' '}
                <span style={{ color: 'var(--kh-text-muted)', fontSize: 'var(--kh-text-sm)' }}>
                  {hit.type === 'product' ? '(محصول)' : '(مقاله)'}
                </span>
                {hit.snippet ? (
                  <p style={{ margin: 0, color: 'var(--kh-text-muted)', fontSize: 'var(--kh-text-sm)' }}>{hit.snippet}</p>
                ) : null}
              </li>
            ))}
          </ul>
        </>
      ) : null}
    </Container>
  )
}

function NotFound() {
  return (
    <Container>
      <StateBlock kind="empty" message="صفحه پیدا نشد." />
      <p style={{ textAlign: 'center' }}>
        <a href="#/">بازگشت به خانه</a>
      </p>
    </Container>
  )
}

// ── shell ─────────────────────────────────────────────────────────────────────

// Local routes stay local: the ecosystem navigation returns absolute production URLs
// (https://shop.khabavar.ir, …), so using it for in-app links would navigate away from
// the dev server and from the SPA. The API nav is used only for the cross-domain
// entries (account, admin), which genuinely live on other hosts.
const LOCAL_NAV = [
  { label: 'خانه', href: '#/' },
  { label: 'فروشگاه', href: '#/shop' },
  { label: 'مجله', href: '#/magazine' },
  { label: 'جست‌وجو', href: '#/search' },
]

function App() {
  const hash = useHashRoute()
  const route = useMemo(() => parseRoute(hash), [hash])
  // The shared navigation comes from the ecosystem module (P7); fall back to local
  // anchors if the API is unreachable so the shell never renders without a nav.
  const nav = useAsync(() => navigation(), [])
  const external = (nav.data?.items ?? [])
    .filter((i) => i.type === 'account' || i.type === 'admin')
    .map((i) => ({ label: i.label, href: i.href }))
  const items = [...LOCAL_NAV, ...external]

  let screen: JSX.Element
  switch (route.name) {
    case 'home':
      screen = <HomeScreen />
      break
    case 'shop':
      screen = <ShopScreen />
      break
    case 'product':
      screen = <ProductScreen id={route.param} />
      break
    case 'magazine':
      screen = <MagazineScreen />
      break
    case 'article':
      screen = <ArticleScreen slug={route.param} />
      break
    case 'search':
      screen = <SearchScreen initial={route.query.get('q') ?? ''} />
      break
    default:
      screen = <NotFound />
  }

  return (
    <div style={{ minHeight: '100%', display: 'flex', flexDirection: 'column' }}>
      <Header items={items} />
      <main style={{ flex: 1 }}>{screen}</main>
      <Footer />
    </div>
  )
}

createRoot(document.getElementById('root')!).render(
  <ErrorBoundary>
    <App />
  </ErrorBoundary>,
)
