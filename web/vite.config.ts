import { defineConfig, loadEnv } from 'vite'

import vue from '@vitejs/plugin-vue'

import tailwindcss from '@tailwindcss/vite'

import path from 'path'



export default defineConfig(({ mode }) => {

  const env = loadEnv(mode, process.cwd(), '')

  // Em produção, o SPA é servido em /app/* pelo mediaserver.

  // O base define o prefixo dos paths dos assets no HTML gerado.

  return {

  plugins: [vue(), tailwindcss()],

  resolve: {

    alias: {

    '@': path.resolve(__dirname, './src'),

    },

  },

  server: {

    port: parseInt(env.VITE_DEV_PORT || '5173'),

    strictPort: true,

    proxy: {

    '/api': {

      target: env.VITE_API_TARGET || 'http://localhost:3000',

      changeOrigin: true,

    },

    },

  },

  build: {

    outDir: 'dist',

    emptyOutDir: true,

  },

  base: '/app/',

  }

})