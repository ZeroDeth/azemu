import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      '/api': {
        target: 'https://localhost:4566',
        secure: false,
        changeOrigin: true,
      },
      '/health': {
        target: 'http://localhost:4568',
        changeOrigin: true,
      },
      '/metadata': {
        target: 'https://localhost:4567',
        secure: false,
        changeOrigin: true,
      },
    },
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
})
