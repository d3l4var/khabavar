import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// RTL Native + Mobile First + Design Tokens (ADR-FE-002)
export default defineConfig({
  plugins: [react()],
  server: { port: 5176 },
  build: { outDir: 'dist' },
})
