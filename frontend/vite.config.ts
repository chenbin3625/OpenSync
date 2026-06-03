import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  server: {
    port: 3000,
    proxy: {
      '/svr': {
        target: 'http://localhost:8023',
        changeOrigin: true,
      },
    },
  },
  build: {
    outDir: '../backend/cmd/server/web',
    emptyOutDir: true,
    sourcemap: false,
  },
})
