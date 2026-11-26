// Stories for the design system. They are the reference for "what this component looks
// like", and the a11y gate runs axe over the built Storybook, so every state below is
// checked for contrast, labels and roles.
import type { Meta, StoryObj } from '@storybook/react-vite'
import {
  ArticleCard,
  Badge,
  Button,
  Card,
  Container,
  Footer,
  Grid,
  Header,
  ProductCard,
  StateBlock,
} from './components'

export default {
  title: 'Design System/Components',
} satisfies Meta

export const Buttons: StoryObj = {
  render: () => (
    <div style={{ display: 'flex', gap: '0.5rem', flexWrap: 'wrap' }}>
      <Button>دکمهٔ اصلی</Button>
      <Button variant="ghost">دکمهٔ ثانویه</Button>
      <Button disabled>غیرفعال</Button>
      <Button href="#x">پیوند</Button>
    </div>
  ),
}

export const Badges: StoryObj = {
  render: () => (
    <div style={{ display: 'flex', gap: '0.5rem', flexWrap: 'wrap' }}>
      <Badge>خنثی</Badge>
      <Badge tone="success">موفق</Badge>
      <Badge tone="warning">هشدار</Badge>
      <Badge tone="error">خطا</Badge>
    </div>
  ),
}

export const Cards: StoryObj = {
  render: () => (
    <Card>
      <h3 style={{ margin: 0 }}>کارت نمونه</h3>
      <p style={{ margin: 0, color: 'var(--kh-text-muted)' }}>توضیح کوتاه داخل کارت.</p>
    </Card>
  ),
}

export const States: StoryObj = {
  render: () => (
    <div style={{ display: 'grid', gap: '1rem' }}>
      <StateBlock kind="loading" />
      <StateBlock kind="empty" message="چیزی برای نمایش نیست." />
      <StateBlock kind="error" message="خطا در دریافت داده." onRetry={() => {}} />
    </div>
  ),
}

export const ProductCards: StoryObj = {
  render: () => (
    <Grid>
      <ProductCard
        product={{ id: '1', title: 'بالش طبی مموری', short_description: 'حمایت از گردن', sleep_relevance: 'SLEEP_RELATED' }}
        href="#p1"
      />
      <ProductCard product={{ id: '2', title: 'پتوی وزن‌دار', short_description: 'خواب عمیق‌تر' }} href="#p2" />
    </Grid>
  ),
}

export const ArticleCards: StoryObj = {
  render: () => (
    <Grid>
      <ArticleCard article={{ title: 'خواب عمیق چگونه شکل می‌گیرد؟', summary: 'مرور چرخه‌های خواب' }} href="#a1" />
    </Grid>
  ),
}

export const PageChrome: StoryObj = {
  render: () => (
    <Container>
      <Header
        items={[
          { label: 'فروشگاه', href: '#/' },
          { label: 'مجله', href: '#/magazine' },
        ]}
      />
      <p>محتوای صفحه بین هدر و فوتر قرار می‌گیرد.</p>
      <Footer />
    </Container>
  ),
}
