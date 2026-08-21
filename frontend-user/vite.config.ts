import { fileURLToPath, URL } from 'node:url'

import tailwindcss from '@tailwindcss/vite'
import vue from '@vitejs/plugin-vue'
import { defineConfig } from 'vite'

export default defineConfig({
  plugins: [vue(), tailwindcss()],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
  server: {
    host: '0.0.0.0',
    port: 5173,
    // 开发态直连后端容器映射出来的端口，生产态由 Nginx 同源反代。
    proxy: {
      '/api': {
        target: 'http://127.0.0.1:27431',
        changeOrigin: true,
      },
      '/healthz': {
        target: 'http://127.0.0.1:27431',
        changeOrigin: true,
      },
    },
  },
  build: {
    target: 'es2022',
    chunkSizeWarningLimit: 1500,
    rollupOptions: {
      output: {
        // G6 体积远大于其余依赖，单独成块可让业务代码改动不失效整包缓存。
        manualChunks: {
          g6: ['@antv/g6'],
          vendor: ['vue', 'pinia'],
        },
      },
    },
  },
})
