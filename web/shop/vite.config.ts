import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// RTL Native + shared design system (ADR-FE-002). `../shared` is outside this root.
export default defineConfig({
  plugins: [react()],
  server: {
    port: 5174,
    fs: { allow: ['..'] },
    proxy: { '/api': { target: 'http://127.0.0.1:8080', changeOrigin: true } },
  },
  build: { outDir: 'dist' },
})
