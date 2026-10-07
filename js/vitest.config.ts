import { defineConfig } from 'vitest/config';

export default defineConfig(({ mode }) => ({
  // The suite imports the package by its name: from the source, and under
  // `--mode dist` from the build a release ships.
  resolve: {
    alias: { '@curly-message/parser': mode === 'dist' ? '/dist/index.js' : '/src/index.ts' },
  },
  test: {
    environment: 'node',
    include: ['tests/specs/**/*.spec.ts'],
  },
}));
