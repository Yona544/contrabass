import { defineConfig, loadEnv } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, '.', '')
  const backendURL = env.CONTRABASS_BACKEND_URL || 'http://localhost:8080'

  return {
    plugins: [react()],
    base: './',
    build: {
      outDir: 'dist',
    },
    server: {
      proxy: {
        '/api': backendURL,
      },
    },
  }
})
