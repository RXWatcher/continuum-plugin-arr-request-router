import { defineConfig } from 'vitest/config';
import react from '@vitejs/plugin-react';
import tailwindcss from '@tailwindcss/vite';

// Continuum mounts each plugin under /api/v1/plugins/{installationId}/, where
// installationId is assigned at install time. Using a relative base ("./")
// makes asset URLs resolve against the current page's path, so the SPA works
// regardless of installation ID.
export default defineConfig({
  base: './',
  plugins: [react(), tailwindcss()],
  build: { outDir: 'dist', emptyOutDir: true },
  test: {
    environment: 'jsdom',
  },
});
