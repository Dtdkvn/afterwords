import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  build: {
    outDir: '../internal/server/webdist',
    emptyOutDir: true,
    sourcemap: true,
  },
  server: {
    port: 5173,
    proxy: {
      '/api': process.env.VITE_PROXY_TARGET || 'http://localhost:8080',
      '/c': process.env.VITE_PROXY_TARGET || 'http://localhost:8080',
      '/healthz': process.env.VITE_PROXY_TARGET || 'http://localhost:8080',
    },
  },
})
