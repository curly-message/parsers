import { defineConfig } from 'tsup';

export default defineConfig(
  /** @type {() => import('tsup').Options} */
  (options) => ({
    clean: true,
    // The types name `Intl.RelativeTimeFormatOptions`, which TypeScript
    // declares in a lib a consumer compiling for an older target does not
    // load; a reference directive in the source is dropped from the bundle,
    // so the bundle is given one.
    dts: { banner: '/// <reference lib="es2020.intl" />' },
    format: ['esm'],
    entry: ['src/index.ts'],
    minify: !options.watch,
    sourcemap: options.watch,
    splitting: true,
  }),
);
