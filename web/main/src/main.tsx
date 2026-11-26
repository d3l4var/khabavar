// KHABAVAR — main entrypoint (scaffold)
// توکن‌ها از Design System (Brand §۹); RTL Native; حالت‌های Loading/Empty/Error مشترک.
// پیاده‌سازی واقعی فاز به فاز طبق IMPLEMENTATION_GUIDE.md.
import { createRoot } from 'react-dom/client'

function App() {
  return <div dir="rtl" lang="fa-IR">خواب‌آور — main (scaffold)</div>
}

createRoot(document.getElementById('root')!).render(<App />)
