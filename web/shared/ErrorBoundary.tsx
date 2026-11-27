// A render error anywhere in the tree unmounts the whole app, which a customer sees as a white
// page — "it shows for a second and then goes blank". React has no default boundary, so one bad
// field (a missing price, a null product) is enough. This keeps the shell and says what happened.
import { Component, type ErrorInfo, type ReactNode } from 'react'

type Props = { children: ReactNode }
type State = { error: Error | null }

export class ErrorBoundary extends Component<Props, State> {
  state: State = { error: null }

  static getDerivedStateFromError(error: Error): State {
    return { error }
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    // Keep the detail in the console for whoever is debugging; the customer gets a sentence.
    console.error('khabavar: render error', error, info.componentStack)
  }

  render() {
    if (!this.state.error) return this.props.children
    return (
      <div
        dir="rtl"
        style={{
          fontFamily: 'system-ui, sans-serif',
          minHeight: '60vh',
          display: 'grid',
          placeItems: 'center',
          padding: '2rem',
          textAlign: 'center',
        }}
      >
        <div>
          <h1 style={{ fontSize: '1.1rem', marginBottom: '.5rem' }}>مشکلی پیش آمد</h1>
          <p style={{ color: '#666', marginBottom: '1rem' }}>
            این بخش بارگذاری نشد. می‌توانید صفحه را دوباره بار کنید.
          </p>
          <button
            onClick={() => window.location.reload()}
            style={{ padding: '.7rem 1.2rem', borderRadius: 8, border: 0, background: '#3b82f6', color: '#fff', font: 'inherit', cursor: 'pointer' }}
          >
            بارگذاری دوباره
          </button>
        </div>
      </div>
    )
  }
}
