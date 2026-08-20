import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import { mkdirSync, writeFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'

const webGitkeepPath = fileURLToPath(new URL('../backend/cmd/server/web/.gitkeep', import.meta.url))

// First-paint / Home overview / Layout primitives. Form, Select, TreeSelect,
// Modal, Input, Table, and DatePicker stay out of this list so lazy routes
// do not inflate the preloaded antd-vendor chunk.
const antdShellModules = [
  '/antd/es/config-provider',
  '/antd/es/app',
  '/antd/es/theme',
  '/antd/es/_util',
  '/antd/es/locale',
  '/antd/es/button',
  '/antd/es/typography',
  '/antd/es/layout',
  '/antd/es/menu',
  '/antd/es/space',
  '/antd/es/tooltip',
  '/antd/es/spin',
  '/antd/es/empty',
  '/antd/es/card',
  '/antd/es/tag',
  '/antd/es/tabs',
  '/antd/es/alert',
  '/antd/es/drawer',
  '/antd/es/pagination',
  '/antd/es/switch',
  '/antd/es/descriptions',
  '/antd/es/popconfirm',
]

function isAntdShell(id: string): boolean {
  return antdShellModules.some((prefix) => id.includes(prefix))
}

function vendorChunk(id: string): string | undefined {
  if (!id.includes('node_modules')) return
  // NAS / Docker installs typically speak HTTP/1.1 (no TLS → no HTTP/2 multiplexing).
  // One request per vendor family beats dozens of 60-byte antd-* chunks whose headers
  // cost more than the payload and stall on the browser's 6-connection cap.
  if (
    id.includes('/react/') ||
    id.includes('/react-dom/') ||
    id.includes('/react-router') ||
    id.includes('/scheduler/')
  ) {
    return 'react-vendor'
  }
  const isTable =
    id.includes('/antd/es/table') ||
    id.includes('/antd/lib/table') ||
    id.includes('/@rc-component/table') ||
    id.includes('/rc-table')
  const isPicker =
    id.includes('/antd/es/date-picker') ||
    id.includes('/antd/lib/date-picker') ||
    id.includes('/antd/es/calendar') ||
    id.includes('/antd/lib/calendar') ||
    id.includes('/antd/es/time-picker') ||
    id.includes('/@rc-component/picker') ||
    id.includes('/rc-picker')
  if (isTable || isPicker) {
    return
  }
  // Do not name a global antd-icons chunk: that disables tree-shaking and
  // statically pulls the full glyph barrel into index.html (same waterfall
  // as antd-picker). Unused icons stay out of the graph; shared glyphs
  // become small async files instead of inflating the preloaded vendor.
  if (id.includes('/@ant-design/icons')) {
    return
  }
  if (isAntdShell(id) || id.includes('/@ant-design/') || id.includes('/dayjs')) {
    return 'antd-vendor'
  }
}

export default defineConfig({
  plugins: [
    react(),
    {
      name: 'preserve-go-embed-placeholder',
      closeBundle() {
        mkdirSync(fileURLToPath(new URL('../backend/cmd/server/web', import.meta.url)), { recursive: true })
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
    target: 'es2022',
    modulePreload: {
      // Native modulepreload is supported by every browser this SPA targets;
      // the Vite polyfill is extra parse work on the critical path.
      polyfill: false,
      resolveDependencies(_filename, deps) {
        // Vite's default preloads every async route onto index.html. Login and
        // the overview tab should not pay for Table / DatePicker / other pages.
        return deps.filter((dep) =>
          dep.includes('react-vendor') ||
          dep.includes('antd-vendor') ||
          dep.includes('rolldown-runtime'),
        )
      },
    },
    chunkSizeWarningLimit: 1400,
    rollupOptions: {
      output: {
        manualChunks: vendorChunk,
      },
    },
  },
})
