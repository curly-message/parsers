import { describe, expect, it } from 'vitest';
import { createParser } from '../../src';
import type { Modifier, Parser, Report } from '../../src';

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

  it('accepts a wrapper carrying any one of its keys, and all three', () => {
    const { resolve } = createParser<{ count: number }>();
    const value: Modifier.Wrapper<number> = { value: 1234.56 };
    const fallback: Modifier.Wrapper<number> = { default: 'none' };
    const props: Modifier.Wrapper<number> = { props: { number: { maximumFractionDigits: 1 } } };
    const every: Modifier.Wrapper<number> = { value: 1234.56, default: 'none', props: { number: { maximumFractionDigits: 1 } } };

    expect(resolve('{{count:number}}', { payload: { count: value }, locale: 'en' })).toBe('1,234.56');
    expect(resolve('{{count:number}}', { payload: { count: fallback }, locale: 'en' })).toBe('none');
    expect(resolve('{{count:number}}', { payload: { count: props }, locale: 'en' })).toBe('');
    expect(resolve('{{count:number}}', { payload: { count: every }, locale: 'en' })).toBe('1,234.6');
  });

  it('accepts a wrapper owning a key that holds no value', () => {
    const { resolve } = createParser<{ count: number }>();
    const count: Modifier.Wrapper<number> = { value: undefined };

    expect(resolve('{{count; default:D}}', { payload: { count } })).toBe('D');
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
        // A modifier is handed the properties its own name holds, composed out
        // of the layers a table declares; the config type still names the
        // table, so a modifier reads its own slice through an assertion.
        'x-temp': ({ value, props }) => `${value}${(props as { unit?: 'C' | 'F' } | undefined)?.unit}`,
      },
      modifierDefaults: { 'x-temp': { unit: 'C' } },
    });

    expect(resolve('{{v:x-temp}}', { payload: { v: '21' } })).toBe('21C');
    expect(resolve('{{v:x-temp}}', { payload: { v: '21' }, props: { 'x-temp': { unit: 'F' } } })).toBe('21F');
  });

  it('accepts a wrapper declaring props for a host-defined modifier', () => {
    const { resolve } = createParser<{ v: string }, { 'x-temp'?: { unit: 'C' | 'F' } }>({
      customModifiers: {
        'x-temp': ({ value, props }) => `${value}${(props as { unit?: 'C' | 'F' } | undefined)?.unit}`,
      },
    });

    expect(resolve('{{v:x-temp}}', { payload: { v: { value: '21', props: { 'x-temp': { unit: 'F' } } } }, props: { 'x-temp': { unit: 'C' } } })).toBe('21F');
  });

  it('rejects a props name a modifier declaring none cannot read', () => {
    const digits: Modifier.T = ({ value, props }) => `${value}@${(props as Intl.NumberFormatOptions | undefined)?.maximumFractionDigits ?? 0}`;
    // @ts-expect-error a modifier that declares no props of its own reads the built-in table, which names no `x-own`
    const own: Modifier.T = ({ value, props }) => `${value}@${props?.['x-own']?.width}`;
    // A modifier reads the slice its own name holds, so the one reading the
    // built-in table's `number` properties is the one registered under that
    // name — a caller's table stands over the built-in it names.
    const { resolve } = createParser({ customModifiers: { number: digits, 'x-own': own } });
    const context = { payload: { v: '1' }, props: { number: { maximumFractionDigits: 3 } } };

    expect(resolve('{{v:number}}', context)).toBe('1@3');
    expect(resolve('{{v:x-own}}', context)).toBe('1@undefined');
  });

  it('rejects a props name a modifier table declaring none cannot read', () => {
    // @ts-expect-error a table declaring no props holds modifiers reading the built-in one, which names no `x-own`
    const table: Modifier.CustomModifiers<'x-own'> = { 'x-own': ({ value, props }) => `${value}@${props?.['x-own']?.width}` };
    const { resolve } = createParser({ customModifiers: table });

    expect(resolve('{{v:x-own}}', { payload: { v: '1' } })).toBe('1@undefined');
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

  it('rejects an entry owning no wrapper key at all', () => {
    const { resolve } = createParser<Payload>();
    // @ts-expect-error an entry owning nothing is a value, which leaves the declared `string`
    const applicationName: Modifier.Wrapper<string> = {};

    expect(resolve(GREETING, { payload: { applicationName } })).toBe('Hi {}!');
  });

  it('rejects wrapper props that are not a table of modifier props', () => {
    const { resolve } = createParser<{ count: number }>();
    // @ts-expect-error a wrapper's `props` is keyed by modifier name, like every other layer
    const count: Modifier.Wrapper<number> = { value: 1234.56, props: 42 };

    expect(resolve('You have {{count:number}}.', { payload: { count }, locale: 'en' })).toBe('You have 1,234.56.');
  });

  it('rejects a wrapper prop option of a type its modifier does not take', () => {
    const { resolve } = createParser<{ count: number }>();
    // @ts-expect-error `maximumFractionDigits` is a number, and a modifier that throws takes its fallback
    const count: Modifier.Wrapper<number> = { value: 1234.56, props: { number: { maximumFractionDigits: 'lots' } } };

    expect(resolve('You have {{count:number}}.', { payload: { count }, locale: 'en' })).toBe('You have .');
  });

  it('rejects wrapper props naming a modifier nobody registered', () => {
    const { resolve } = createParser<{ count: number }>();
    // @ts-expect-error `nope` is no registered modifier, so no layer carries props for it
    const count: Modifier.Wrapper<number> = { value: 1234.56, props: { nope: { a: 1 } } };

    expect(resolve('You have {{count:number}}.', { payload: { count }, locale: 'en' })).toBe('You have 1,234.56.');
  });
});

// `Parser.Options` is published, so it has to read like one a consumer can
// take apart: indexing it for a single option and taking the bag as a
// parameter are the ordinary uses.
describe('parser option typing', () => {
  it('names a single option and reads it off the bag', () => {
    const reports: Report[] = [];
    const onReport: Parser.Options['onReport'] = (report) => { reports.push(report); };
    const readHandler = (options: Parser.Options) => options.onReport;
    const options = { onReport };
    const { resolve } = createParser(options);

    expect(readHandler(options)).toBe(onReport);
    expect(resolve('{{v:nosuch; default:D}}', { payload: { v: 'X' } })).toBe('D');
    expect(reports.map(({ code }) => code)).toEqual(['unknown-modifier']);
  });

  it('rejects an option bag carrying a key the parser does not read', () => {
    // @ts-expect-error the parser reads `customModifiers`, `modifierDefaults` and `onReport`
    const { resolve } = createParser({ onreport: () => {} });

    expect(resolve('Hi!')).toBe('Hi!');
  });

  it('rejects props names a bag declaring none cannot read', () => {
    const options: Parser.Options = {
      // @ts-expect-error a bag declaring no props holds modifiers reading the built-in table, which names no `x-own`
      customModifiers: { 'x-own': ({ value, props }) => `${value}@${props?.['x-own']?.width}` },
      // @ts-expect-error and carries defaults for that same table, which names no `x-own` either
      modifierDefaults: { 'x-own': { width: 3 } },
    };
    const { resolve } = createParser(options);

    expect(resolve('{{v:x-own}}', { payload: { v: '1' } })).toBe('1@undefined');
  });
});

// `Modifier.DefaultKeys` is `keyof typeof modifiers`, so the modifier module's
// exports are the registry — a helper exported beside the modifiers would
// answer to its own name in a message. This is the type-level half of the
// tripwire; `the modifier registry holds modifiers alone` is the runtime half.
describe('modifier typing', () => {
  it('names the registered modifiers, and nothing else', () => {
    type Exact<A, B> = [A] extends [B] ? [B] extends [A] ? true : never : never;
    type Registered = 'eq' | 'ne' | 'lt' | 'gt' | 'lte' | 'gte' | 'number' | 'date' | 'ago' | 'currency';

    const registered: Exact<Modifier.DefaultKeys, Registered> = true;

    expect(registered).toBe(true);
  });
});
