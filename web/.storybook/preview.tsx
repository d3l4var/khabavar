import '../shared/tokens.css'

// Every app is fa-IR / RTL (ADR-FE-002), so the canvas must be too — a component that
// looks fine in LTR can break in the real product.
const preview = {
  parameters: {
    layout: 'centered',
    backgrounds: { default: 'dark', values: [{ name: 'dark', value: '#0f1115' }] },
  },
  decorators: [
    (Story: () => JSX.Element) => (
      <div dir="rtl" lang="fa" style={{ padding: '1rem', minWidth: '320px' }}>
        <Story />
      </div>
    ),
  ],
}

export default preview
