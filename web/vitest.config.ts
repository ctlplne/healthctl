import { defineConfig } from 'vitest/config'

export default defineConfig({
  // The app imports shared notice text from ../internal/clinical.
  server: { fs: { allow: ['..'] } },
  test: {
    environment: 'jsdom',
    include: ['src/**/*.test.{ts,tsx}'],
  },
})
