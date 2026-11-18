import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { fileURLToPath, URL } from 'node:url'

// 构建产物直接输出到 Go 的 embed 目录 internal/gui/web/，
// 这样前端源码保留在 /front，产物随 Go 一起编译进单二进制。
const outDir = fileURLToPath(new URL('../internal/gui/web', import.meta.url))

export default defineConfig({
  base: '/',
  plugins: [vue()],
  build: {
    outDir,
    emptyOutDir: true, // 完整重建；login.html/session.js/favicon 由 public/ 自动复制
    rollupOptions: {
      output: {
        entryFileNames: 'assets/app.[hash].js',
        chunkFileNames: 'assets/chunk.[hash].js',
        assetFileNames: 'assets/app.[hash][extname]'
      }
    }
  }
})
