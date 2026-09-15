import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import { existsSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'

const backendWebDir = fileURLToPath(new URL('../backend/cmd/server/web', import.meta.url))
const webGitkeepPath = `${backendWebDir}/.gitkeep`
const packageJson = JSON.parse(
  readFileSync(new URL('./package.json', import.meta.url), 'utf8')
) as { version: string }

export default defineConfig({
  define: {
    'import.meta.env.PACKAGE_VERSION': JSON.stringify(packageJson.version),
  },
  plugins: [
    react(),
    tailwindcss(),
    {
      name: 'preserve-go-embed-placeholder',
      closeBundle() {
        // emptyOutDir wipes the go:embed placeholder, restore it — but only
        // when the backend tree actually exists next to the frontend (an
        // independent frontend checkout must not sprout a backend/ directory).
        if (!existsSync(backendWebDir)) return
        mkdirSync(backendWebDir, { recursive: true })
        writeFileSync(webGitkeepPath, '')
      },
    },
  ],
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
          if (
            id.includes('/node_modules/react/') ||
            id.includes('/node_modules/react-dom/') ||
            id.includes('/node_modules/scheduler/')
          ) {
            return 'react-vendor'
          }
          if (id.includes('/@radix-ui/')) {
            return 'radix-vendor'
          }
          if (id.includes('/lucide-react/')) {
            return 'icons-vendor'
          }
          if (id.includes('/dayjs/')) {
            return 'dayjs-vendor'
          }
        },
      },
    },
  },
})
