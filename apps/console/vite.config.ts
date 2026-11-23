import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

// 开发环境将 /api 代理到 Go 服务（默认 :8080）。
export default defineConfig({
  plugins: [vue()],
  server: {
    proxy: {
      '/api': {
        target: process.env.VITE_DEV_PROXY_TARGET || 'http://127.0.0.1:8080',
        changeOrigin: true
      }
    }
  },
  build: {
    outDir: 'dist',
    sourcemap: false,
    chunkSizeWarningLimit: 1600
  }
})