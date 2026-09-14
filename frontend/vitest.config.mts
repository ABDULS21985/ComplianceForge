import { fileURLToPath, URL } from 'node:url';

import react from '@vitejs/plugin-react';
import { defineConfig } from 'vitest/config';

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
  test: {
    environment: 'node',
    include: ['src/**/*.test.{ts,tsx}'],
    exclude: ['e2e/**', 'node_modules/**', '.next/**', 'playwright-report/**'],
    passWithNoTests: false,
    restoreMocks: true,
    clearMocks: true,
    coverage: {
      reporter: ['text', 'json-summary', 'html'],
      reportsDirectory: './coverage',
      include: ['src/lib/**/*.{ts,tsx}', 'src/middleware.ts'],
      exclude: ['src/**/*.test.{ts,tsx}'],
    },
  },
});
