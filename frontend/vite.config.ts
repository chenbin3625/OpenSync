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
    rollupOptions: {
      output: {
        manualChunks(id) {
          if (!id.includes('node_modules')) return
          if (id.includes('/react') || id.includes('/react-dom') || id.includes('/scheduler')) {
            return 'react-vendor'
          }
          if (id.includes('/@ant-design/icons')) {
            return 'icons-vendor'
          }
          const antdMatch = id.match(/node_modules\/antd\/(?:es|lib)\/([^/]+)/)
          if (antdMatch) {
            return `antd-${antdMatch[1]}`
          }
          const rcMatch = id.match(/node_modules\/(rc-[^/]+)/)
          if (rcMatch) {
            return rcMatch[1]
          }
          if (id.includes('/dayjs/')) {
            return 'dayjs-vendor'
          }
        },
      },
    },
  },
})
