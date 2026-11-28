// KHABAVAR — magazine (magazine.khabavar.ir).
//
// Editorial content only: published articles and the topic tree. Every article shown
// here passed the publish gate (at least one source, ADR-CONTENT-001) — the API
// enforces it, so the client never has to filter by status.
import { useEffect, useMemo, useState } from 'react'
import { createRoot } from 'react-dom/client'
import { Badge, Button, Card, Container, Footer, Grid, Header, StateBlock, useAsync } from '../../shared/components'
import { getArticle, getArticleProducts, listArticles, listArticlesByTopic, listTopics, type Article, type Topic } from '../../shared/api'
import { ErrorBoundary } from '../../shared/ErrorBoundary'
import '../../shared/tokens.css'

const NAV = [
  { label: 'مجله', href: '#/' },
  { label: 'موضوع‌ها', href: '#/topics' },
  { label: 'فروشگاه', href: 'https://shop.khabavar.ir' },
  { label: 'خانه', href: 'https://khabavar.ir' },
]

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
  if (segs.length === 0) return { name: 'home', param: '' }
  if (segs[0] === 'article' && segs[1]) return { name: 'article', param: decodeURIComponent(segs[1]) }
  if (segs[0] === 'topic' && segs[1]) return { name: 'topic', param: decodeURIComponent(segs[1]) }
  return { name: segs[0], param: segs[1] ?? '' }
}

function faDate(iso?: string) {
  if (!iso) return ''
  try {
    return new Intl.DateTimeFormat('fa-IR', { dateStyle: 'medium' }).format(new Date(iso))
  } catch {
    return iso.slice(0, 10)
  }
}

function ArticleCard({ a }: { a: Article }) {
  return (
    <Card>
      <h3 style={{ margin: 0, fontSize: 'var(--kh-text-lg)' }}>
        <a href={`#/article/${encodeURIComponent(a.slug)}`} style={{ color: 'var(--kh-text)' }}>
          {a.title}
        </a>
      </h3>
      {a.summary ? (
        <p style={{ margin: 'var(--kh-space-2) 0 0', color: 'var(--kh-text-muted)', fontSize: 'var(--kh-text-sm)' }}>{a.summary}</p>
      ) : null}
      <p style={{ margin: 'var(--kh-space-2) 0 0', color: 'var(--kh-text-muted)', fontSize: 'var(--kh-text-xs)' }}>
        {faDate(a.published_at)}
      </p>
    </Card>
  )
}

function TopicChips({ topics }: { topics: Topic[] }) {
  return (
    <div style={{ display: 'flex', gap: 'var(--kh-space-2)', flexWrap: 'wrap', margin: 'var(--kh-space-4) 0' }}>
      {topics.map((t) => (
        <Button key={t.id} href={`#/topic/${encodeURIComponent(t.id)}`} variant="ghost">
          {t.name}
        </Button>
      ))}
    </div>
  )
}

function HomeScreen() {
  const articles = useAsync(() => listArticles(1, 12), [])
  const topics = useAsync(() => listTopics(), [])

  return (
    <Container>
      <h1 style={{ marginTop: 'var(--kh-space-6)', fontSize: 'var(--kh-text-2xl)' }}>مجلهٔ خواب</h1>
      <p style={{ color: 'var(--kh-text-muted)' }}>
        نوشته‌های مبتنی بر منبع دربارهٔ خواب، بی‌خوابی و انتخاب محصول. محتوای مجله جایگزین توصیهٔ پزشکی نیست.
      </p>

      {topics.data && (topics.data.data ?? []).length > 0 ? <TopicChips topics={topics.data.data ?? []} /> : null}

      <h2 style={{ marginTop: 'var(--kh-space-5)' }}>تازه‌ترین مقالات</h2>
      {articles.loading ? <StateBlock kind="loading" /> : null}
      {articles.error ? <StateBlock kind="error" message={articles.error} onRetry={articles.retry} /> : null}
      {articles.data && (articles.data.data ?? []).length === 0 ? (
        <StateBlock kind="empty" message="هنوز مقاله‌ای منتشر نشده است." />
      ) : null}
      {articles.data && (articles.data.data ?? []).length > 0 ? (
        <Grid>
          {(articles.data.data ?? []).map((a) => (
            <ArticleCard key={a.id} a={a} />
          ))}
        </Grid>
      ) : null}
    </Container>
  )
}

function TopicsScreen() {
  const topics = useAsync(() => listTopics(), [])
  return (
    <Container>
      <h1 style={{ marginTop: 'var(--kh-space-5)' }}>موضوع‌ها</h1>
      {topics.loading ? <StateBlock kind="loading" /> : null}
      {topics.error ? <StateBlock kind="error" message={topics.error} onRetry={topics.retry} /> : null}
      {topics.data && (topics.data.data ?? []).length > 0 ? (
        <div style={{ display: 'grid', gap: 'var(--kh-space-2)', maxWidth: '520px' }}>
          {(topics.data.data ?? []).map((t) => (
            <Card key={t.id}>
              <a href={`#/topic/${encodeURIComponent(t.id)}`} style={{ color: 'var(--kh-text)' }}>
                {t.name}
              </a>
              <span style={{ color: 'var(--kh-text-muted)', fontSize: 'var(--kh-text-sm)' }}> /{t.slug}</span>
            </Card>
          ))}
        </div>
      ) : null}
    </Container>
  )
}

function TopicScreen({ id }: { id: string }) {
  const articles = useAsync(() => listArticlesByTopic(id, 1, 20), [id])
  const topics = useAsync(() => listTopics(), [])
  const topic = (topics.data?.data ?? []).find((t) => t.id === id)

  return (
    <Container>
      <p style={{ marginTop: 'var(--kh-space-5)' }}>
        <a href="#/">‹ مجله</a>
      </p>
      <h1>{topic ? topic.name : 'موضوع'}</h1>
      {articles.loading ? <StateBlock kind="loading" /> : null}
      {articles.error ? <StateBlock kind="error" message={articles.error} onRetry={articles.retry} /> : null}
      {articles.data && (articles.data.data ?? []).length === 0 ? (
        <StateBlock kind="empty" message="در این موضوع هنوز مقاله‌ای منتشر نشده است." />
      ) : null}
      {articles.data && (articles.data.data ?? []).length > 0 ? (
        <Grid>
          {(articles.data.data ?? []).map((a) => (
            <ArticleCard key={a.id} a={a} />
          ))}
        </Grid>
      ) : null}
    </Container>
  )
}

function ArticleScreen({ slug }: { slug: string }) {
  const article = useAsync(() => getArticle(slug), [slug])
  // The reason a magazine sits beside a shop: send the reader to the product the article is
  // about. The relation rows existed in the database and nothing ever showed them.
  const products = useAsync(() => getArticleProducts(slug), [slug])
  if (article.loading) return <Container><StateBlock kind="loading" /></Container>
  if (article.error) return <Container><StateBlock kind="error" message={article.error} onRetry={article.retry} /></Container>
  if (!article.data) return <Container><StateBlock kind="empty" message="مقاله پیدا نشد." /></Container>

  const a = article.data.data
  // The body is plain text from the editorial pipeline; render it as paragraphs rather
  // than as HTML so an editor cannot inject markup into the public page.
  const paragraphs = (a.body ?? '').split(/\n{2,}/).filter(Boolean)

  return (
    <Container>
      <p style={{ marginTop: 'var(--kh-space-5)' }}>
        <a href="#/">‹ مجله</a>
      </p>
      <article style={{ maxWidth: '68ch' }}>
        <h1 style={{ fontSize: 'var(--kh-text-2xl)' }}>{a.title}</h1>
        <p style={{ color: 'var(--kh-text-muted)', fontSize: 'var(--kh-text-sm)' }}>
          {faDate(a.published_at)} · <Badge tone="neutral">{a.source_type}</Badge>
        </p>
        {a.summary ? <p style={{ fontSize: 'var(--kh-text-lg)', color: 'var(--kh-text-muted)' }}>{a.summary}</p> : null}
        {paragraphs.map((p, i) => (
          <p key={i} style={{ lineHeight: 2 }}>
            {p}
          </p>
        ))}
        {(products.data?.data ?? []).length > 0 ? (
          <section style={{ marginTop: 'var(--kh-space-5)', borderTop: '1px solid var(--kh-border, #e5e5e5)', paddingTop: 'var(--kh-space-4)' }}>
            <h2 style={{ fontSize: 'var(--kh-text-lg)' }}>محصولات مرتبط</h2>
            <ul style={{ listStyle: 'none', padding: 0, display: 'grid', gap: 'var(--kh-space-3)' }}>
              {(products.data?.data ?? []).map((p) => (
                <li key={p.product_id}>
                  <Card>
                    <strong>{p.title}</strong>
                    {p.sales_pitch ? (
                      <p style={{ margin: 'var(--kh-space-1) 0', color: 'var(--kh-text-muted)', fontSize: 'var(--kh-text-sm)' }}>
                        {p.sales_pitch}
                      </p>
                    ) : null}
                    {p.relation_reason ? (
                      <p style={{ margin: 0, color: 'var(--kh-text-muted)', fontSize: 'var(--kh-text-sm)' }}>
                        {p.relation_reason}
                      </p>
                    ) : null}
                    <a href={`https://shop.khabavar.ir/#/product/${encodeURIComponent(p.product_id)}`}>
                      مشاهده در فروشگاه
                    </a>
                  </Card>
                </li>
              ))}
            </ul>
          </section>
        ) : null}
      </article>
      <p style={{ marginTop: 'var(--kh-space-5)', color: 'var(--kh-text-muted)', fontSize: 'var(--kh-text-sm)' }}>
        محتوای مجله جایگزین توصیهٔ پزشکی نیست.
      </p>
    </Container>
  )
}

function App() {
  const hash = useHashRoute()
  const route = useMemo(() => parseRoute(hash), [hash])

  let screen: JSX.Element
  switch (route.name) {
    case 'article':
      screen = <ArticleScreen slug={route.param} />
      break
    case 'topic':
      screen = <TopicScreen id={route.param} />
      break
    case 'topics':
      screen = <TopicsScreen />
      break
    default:
      screen = <HomeScreen />
  }

  return (
    <div style={{ minHeight: '100%', display: 'flex', flexDirection: 'column' }}>
      <Header items={NAV} />
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
