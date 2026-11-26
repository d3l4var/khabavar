import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// RTL Native + Mobile First + Design Tokens (ADR-FE-002).
// `../shared` holds the design system and the API client, shared by all four apps;
// it sits outside this project root, so the dev server must be allowed to read it.
export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    fs: { allow: ['..'] },
    proxy: { '/api': { target: 'http://127.0.0.1:8080', changeOrigin: true } },
  },
  build: { outDir: 'dist' },
})
