import { describe, expect, it } from 'vitest';
import { createParser } from '../../src';
import type { Modifier, Parser } from '../../src';

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

  it('accepts a wrapper where a declared payload type names a value', () => {
    const { resolve } = createParser<Payload>();

    expect(resolve(GREETING, { payload: { applicationName: { value: 'App' } } })).toBe('Hi App!');
  });

  it('accepts a wrapper where no payload type is declared', () => {
    const { resolve } = createParser();

    expect(resolve(GREETING, { payload: { applicationName: { value: 'App' } } })).toBe('Hi App!');
  });

  it('accepts a wrapper carrying a default and no value', () => {
    const { resolve } = createParser<Payload>();

    expect(resolve(GREETING, { payload: { applicationName: { default: 'Guest' } } })).toBe('Hi Guest!');
  });

  it('accepts a payload entry declared apart from the call', () => {
    const { resolve } = createParser<Payload>();
    const applicationName: Parser.PayloadEntry<string> = { value: 'App', default: 'Guest' };

    expect(resolve(GREETING, { payload: { applicationName } })).toBe('Hi App!');
  });

  it('accepts a wrapper declaring the props its modifier reads', () => {
    const { resolve } = createParser<{ count: number }>();
    const count: Modifier.Wrapper<number> = { value: 1234.56, props: { number: { maximumFractionDigits: 1 } } };

    expect(resolve('You have {{count:number}}.', { payload: { count }, locale: 'en' })).toBe('You have 1,234.6.');
  });

  it('rejects an `ago` format the resolution ladder does not walk', () => {
    const { resolve } = createParser<{ v: number }>();
    const payload = { v: -40 * 24 * 60 * 60 * 1000 };

    expect(resolve('{{v:ago}}', { payload, props: { ago: { format: 'month' } }, locale: 'en' })).toBe('last month');
    expect(resolve('{{v:ago}}', { payload, props: { ago: { format: 'days' } }, locale: 'en' })).toBe('40 days ago');

    // @ts-expect-error `quarter` is a unit `Intl` knows and this ladder does not walk
    expect(resolve('{{v:ago}}', { payload, props: { ago: { format: 'quarter' } }, locale: 'en' })).toBe('this year');
  });

  it('accepts implementation defaults for a host-defined modifier', () => {
    const { resolve } = createParser<{ v: string }, { 'x-temp'?: { unit: 'C' | 'F' } }>({
      customModifiers: {
        'x-temp': ({ value, props, parserOptions }) => {
          const { unit } = { ...parserOptions?.modifierDefaults?.['x-temp'], ...props?.['x-temp'] };

          return `${value}${unit}`;
        },
      },
      modifierDefaults: { 'x-temp': { unit: 'C' } },
    });

    expect(resolve('{{v:x-temp}}', { payload: { v: '21' } })).toBe('21C');
    expect(resolve('{{v:x-temp}}', { payload: { v: '21' }, props: { 'x-temp': { unit: 'F' } } })).toBe('21F');
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

  it('rejects a wrapper value of a type the payload does not declare', () => {
    const { resolve } = createParser<Payload>();

    // @ts-expect-error a wrapper carries the value its key declares, and that is a `string`
    expect(resolve(GREETING, { payload: { applicationName: { value: 42 } } })).toBe('Hi 42!');
  });

  it('rejects an entry owning a key no wrapper declares', () => {
    const { resolve } = createParser<Payload>();

    // @ts-expect-error `unit` is no wrapper key, which leaves the declared `string`
    expect(resolve(GREETING, { payload: { applicationName: { value: 'App', unit: 'kg' } } })).toBe('Hi {"value":"App","unit":"kg"}!');
  });
});
