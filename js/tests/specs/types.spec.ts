import { describe, expect, it } from 'vitest';
import { createParser } from '../../src';

const GREETING = 'Hi {{applicationName}}!';

type Payload = { applicationName: string };

// Resolution has no host to stand up, so these run for real. The negative
// cases are compile-time only — `tsc --noEmit`, run by pretest, is what
// makes them assertions.
describe('payload typing', () => {
  it('accepts a named payload key when no payload type is declared', () => {
    const { resolve } = createParser();

    expect(resolve(GREETING, { payload: { applicationName: 'App' } })).toBe('Hi App!');
  });

  it('accepts a payload declared apart from the call', () => {
    const { resolve } = createParser();
    const payload = { applicationName: 'App' };

    expect(resolve(GREETING, { payload })).toBe('Hi App!');
  });

  it('accepts a declared payload type', () => {
    const { resolve } = createParser<Payload>();

    expect(resolve(GREETING, { payload: { applicationName: 'App' } })).toBe('Hi App!');
  });

  it('keeps the `default` payload key', () => {
    const { resolve } = createParser();

    expect(resolve(undefined, { payload: { default: 'FALLBACK' }, key: 'greeting' })).toBe('FALLBACK');
  });

  it('resolves to the key when the message is missing', () => {
    const { resolve } = createParser();

    expect(resolve(undefined, { key: 'greeting' })).toBe('greeting');
  });

  it('resolves without a context', () => {
    const { resolve } = createParser();

    expect(resolve('Hi!')).toBe('Hi!');
  });

  it('rejects a typo against a declared payload type', () => {
    const { resolve } = createParser<Payload>();

    // @ts-expect-error `aplicationName` is not a key of the declared payload
    expect(resolve(GREETING, { payload: { aplicationName: 'App' } })).toBe('Hi !');
  });

  it('rejects a context key resolution does not read', () => {
    const { resolve } = createParser();

    // @ts-expect-error resolution reads the payload, the props, the locale and the key
    expect(resolve(GREETING, { payload: { applicationName: 'App' }, extra: true })).toBe('Hi App!');
  });
});
