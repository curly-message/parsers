import { describe, expect, it } from 'vitest';
import { createParser, Parser, Report } from '../../src';
import { MESSAGES } from '../data';

const defaultLocale = 'en';
const altLocale = 'cs';

const message = (locale: string, key: string) => {
  const [namespace, ...path] = key.split('.');

  return MESSAGES[locale]?.[namespace]?.[path.join('.')];
};

const defaultParser = createParser({
  customModifiers: {
    test: ({ value }) => value,
  },
});

const resolverFor = <P = Parser.PayloadDefault>(locale: string, { resolve }: Parser.T = defaultParser) => (key: string, payload?: Parser.Payload<P>, props?: Parser.Context['props']): string => resolve(message(locale, key), { payload, props, locale, key });

describe('parser', () => {
  it('returns a key string if not defined', () => {
    const resolve = resolverFor<{ value?: any }>(defaultLocale);

    expect(resolve('common.undefined')).toBe('common.undefined');
  });
  it('resolves every message to a string', () => {
    const { resolve } = defaultParser;

    expect(resolve(42)).toBe('42');
    expect(resolve(null)).toBe('null');
    expect(resolve(true)).toBe('true');
    expect(resolve(undefined)).toBe('');
  });
  it('key returns proper value', () => {
    const resolve = resolverFor<{ value?: any }>(defaultLocale);

    expect(resolve('common.no_placeholder')).toBe('NO_PLACEHOLDER');
  });
  it('placeholders work', () => {
    const resolve = resolverFor<{ value?: any }>(defaultLocale);

    expect(resolve('common.placeholder', { value: 'TEST_VALUE' })).toBe('VALUES: TEST_VALUE, TEST_VALUE, TEST_VALUE, TEST_VALUE');
  });
  it('placeholders in payload work', () => {
    const resolve = resolverFor<{ value?: any, another: string }>(defaultLocale);

    expect(resolve('common.placeholder', { value: 'TEST_{{another}}', another: 'VALUE' })).toBe('VALUES: TEST_VALUE, TEST_VALUE, TEST_VALUE, TEST_VALUE');
  });
  it('default value works for placeholders', () => {
    const resolve = resolverFor<{ value?: any }>(defaultLocale);

    expect(resolve('common.placeholder_default')).toBe('VALUES: DEFAULT_VALUE, DEFAULT_VALUE, DEFAULT_VALUE, DEFAULT_VALUE');
  });
  it('dynamic default works for placeholders', () => {
    const resolve = resolverFor<{ value?: any }>(defaultLocale);

    expect(resolve('common.placeholder_unknown', { default: 'DYNAMIC_DEFAULT_VALUE' })).toBe('DYNAMIC_DEFAULT_VALUE');
  });
  it('reads a falsy own `default` as present', () => {
    const { resolve } = defaultParser;

    expect(resolve(undefined, { payload: { default: 0 }, key: 'greeting' })).toBe('0');
    expect(resolve(undefined, { payload: { default: '' }, key: 'greeting' })).toBe('');
    expect(resolve(undefined, { payload: { default: false }, key: 'greeting' })).toBe('false');

    expect(resolve('{{count}}', { payload: { default: 0 } })).toBe('0');
    expect(resolve('{{count}}', { payload: { default: false } })).toBe('false');
    expect(resolve('{{count:number}}', { payload: { count: 'not a number', default: 0 }, locale: 'en' })).toBe('0');
    expect(resolve('{{count; default:INLINE}}', { payload: { default: 0 } })).toBe('0');
  });
  it('an own payload `default` overrides an inline one', () => {
    const { resolve } = defaultParser;

    expect(resolve('{{count; default:INLINE}}', { payload: {} })).toBe('INLINE');
    expect(resolve('{{count; default:INLINE}}', { payload: { default: 'PAYLOAD' } })).toBe('PAYLOAD');
    expect(resolve('{{count; default:INLINE}}', { payload: { count: 7, default: 'PAYLOAD' } })).toBe('7');
    expect(resolve('Hi {{name; default:friend}}, you have {{count; default:no}} messages', { payload: { name: 'Ann', default: '-' } })).toBe('Hi Ann, you have - messages');

    const inherited = Object.create({ default: 'INHERITED' });

    expect(resolve('{{count; default:INLINE}}', { payload: inherited })).toBe('INLINE');
  });
  it('placeholders containing escaped values work', () => {
    const resolve = resolverFor<{ 'pl:ace;holder'?: any }>(defaultLocale);

    expect(resolve('common.placeholder_escaped', { 'pl:ace;holder': 'TEST \\{\\{VALUE\\}\\}' })).toBe('TEST {{VALUE}}');
  });
  it('`eq` modifier works', () => {
    const resolve = resolverFor<{ value?: any }>(defaultLocale);

    expect(resolve('common.modifier_eq', { value: 'option9' })).toBe('VALUES: DEFAULT VALUE, DEFAULT VALUE, DEFAULT VALUE, DEFAULT VALUE');
    expect(resolve('common.modifier_eq', { value: 'option2' })).toBe('VALUES: VALUE2, VALUE2, VALUE2, VALUE2');
    expect(resolve('common.modifier_eq')).toBe('VALUES: DEFAULT VALUE, DEFAULT VALUE, DEFAULT VALUE, DEFAULT VALUE');
  });
  it('`ne` modifier works', () => {
    const resolve = resolverFor<{ value?: any }>(defaultLocale);

    expect(resolve('common.modifier_ne', { value: 10 })).toBe('DEFAULT VALUE');
    expect(resolve('common.modifier_ne', { value: 5 })).toBe('VALUE2');
    expect(resolve('common.modifier_ne', { value: 15 })).toBe('VALUE2');
    expect(resolve('common.modifier_ne')).toBe('VALUE2');
  });
  it('`lt` modifier works', () => {
    const resolve = resolverFor<{ value?: any }>(defaultLocale);

    expect(resolve('common.modifier_lt', { value: 10 })).toBe('DEFAULT VALUE');
    expect(resolve('common.modifier_lt', { value: 5 })).toBe('VALUE2');
    expect(resolve('common.modifier_lt')).toBe('DEFAULT VALUE');
  });
  it('`lte` modifier works', () => {
    const resolve = resolverFor<{ value?: any }>(defaultLocale);

    expect(resolve('common.modifier_lte', { value: 10 })).toBe('VALUE2');
    expect(resolve('common.modifier_lte', { value: 5 })).toBe('VALUE2');
    expect(resolve('common.modifier_lte')).toBe('DEFAULT VALUE');
  });
  it('`gt` modifier works', () => {
    const resolve = resolverFor<{ value?: any }>(defaultLocale);

    expect(resolve('common.modifier_gt', { value: 10 })).toBe('VALUE1');
    expect(resolve('common.modifier_gt', { value: 15 })).toBe('VALUE2');
    expect(resolve('common.modifier_gt')).toBe('DEFAULT VALUE');
  });
  it('`gte` modifier works', () => {
    const resolve = resolverFor<{ value?: any }>(defaultLocale);

    expect(resolve('common.modifier_gte', { value: 10 })).toBe('VALUE2');
    expect(resolve('common.modifier_gte', { value: 15 })).toBe('VALUE2');
    expect(resolve('common.modifier_gte')).toBe('DEFAULT VALUE');
  });
  it('`number` modifier works', () => {
    const resolve = resolverFor<{ value?: any }>(defaultLocale);
    const resolveAlt = resolverFor<{ value?: any }>(altLocale);
    const value = 123456.789;

    expect(resolve('common.modifier_number', { value })).toBe(new Intl.NumberFormat(defaultLocale, { maximumFractionDigits: 2 }).format(value));
    expect(resolveAlt('common.modifier_number', { value })).toBe(new Intl.NumberFormat(altLocale, { maximumFractionDigits: 2 }).format(value));
  });
  it('`number` props work', () => {
    const resolve = resolverFor<{ value?: any }>(defaultLocale);
    const value = 123456.78987686643;

    expect(resolve('common.modifier_number', { value }, { number: { maximumFractionDigits: 4 } })).toBe(new Intl.NumberFormat(defaultLocale, { maximumFractionDigits: 4 }).format(value));
  });
  it('`number` defaults work', () => {
    const resolve = resolverFor<{ value?: any }>(defaultLocale, createParser({ modifierDefaults: { number: { maximumFractionDigits: 4 } } }));
    const value = 123456.78987686643;

    expect(resolve('common.modifier_number', { value })).toBe(new Intl.NumberFormat(defaultLocale, { maximumFractionDigits: 4 }).format(value));
  });
  it('a call prop set to `undefined` leaves `modifierDefaults` standing', () => {
    const { resolve } = createParser({ modifierDefaults: { number: { maximumFractionDigits: 4, useGrouping: false } } });
    const value = 1234.56789;

    expect(resolve('{{v:number}}', { payload: { v: value }, props: { number: { useGrouping: undefined, maximumFractionDigits: undefined } }, locale: defaultLocale })).toBe('1234.5679');
  });
  it('`date` modifier works', () => {
    const resolve = resolverFor<{ value?: any }>(defaultLocale);
    const resolveAlt = resolverFor<{ value?: any }>(altLocale);
    const value = Date.now();

    expect(resolve('common.modifier_date', { value })).toBe(new Intl.DateTimeFormat(defaultLocale, {}).format(value));
    expect(resolveAlt('common.modifier_date', { value })).toBe(new Intl.DateTimeFormat(altLocale, {}).format(value));
  });
  it('`date` props work', () => {
    const resolve = resolverFor<{ value?: any }>(defaultLocale);
    const value = Date.now();

    expect(resolve('common.modifier_date', { value }, { date: { year: '2-digit', month: 'numeric', day: 'numeric' } })).toBe(new Intl.DateTimeFormat(defaultLocale, { year: '2-digit', month: 'numeric', day: 'numeric' }).format(value));
  });
  it('`date` defaults work', () => {
    const resolve = resolverFor<{ value?: any }>(defaultLocale, createParser({ modifierDefaults: { date: { timeStyle: 'full' } } }));
    const value = Date.now();

    expect(resolve('common.modifier_date', { value })).toBe(new Intl.DateTimeFormat(defaultLocale, { timeStyle: 'full' }).format(value));
  });
  it('`ago` modifier works', () => {
    const resolve = resolverFor<{ value?: any }>(defaultLocale);
    const resolveAlt = resolverFor<{ value?: any }>(altLocale);
    const value = -1000 * 60 * 30;

    expect(resolve('common.modifier_ago', { value })).toBe(new Intl.RelativeTimeFormat(defaultLocale).format(-30, 'minute'));
    expect(resolveAlt('common.modifier_ago', { value })).toBe(new Intl.RelativeTimeFormat(altLocale).format(-30, 'minute'));
  });
  it('`ago` props work', () => {
    const resolve = resolverFor<{ value?: any }>(defaultLocale);
    const value = -1000 * 60 * 60 * 24 * 7;

    expect(resolve('common.modifier_ago', { value }, { ago: { format: 'day' } })).toBe(new Intl.RelativeTimeFormat(defaultLocale).format(-7, 'day'));
    expect(resolve('common.modifier_ago', { value }, { ago: { format: 'week' } })).not.toBe(new Intl.RelativeTimeFormat(defaultLocale).format(-7, 'day'));
  });
  it('`ago` defaults work', () => {
    const resolveDays = resolverFor<{ value?: any }>(defaultLocale, createParser({ modifierDefaults: { ago: { format: 'days' } } }));
    const resolveWeek = resolverFor<{ value?: any }>(defaultLocale, createParser({ modifierDefaults: { ago: { format: 'week' } } }));
    const value = -1000 * 60 * 60 * 24 * 7;

    expect(resolveDays('common.modifier_ago', { value })).toBe(new Intl.RelativeTimeFormat(defaultLocale).format(-7, 'day'));
    expect(resolveWeek('common.modifier_ago', { value })).not.toBe(new Intl.RelativeTimeFormat(defaultLocale).format(-7, 'day'));
  });
  it('`currency` modifier works', () => {
    const resolve = resolverFor<{ value?: number }>(defaultLocale);
    const value = 10;
    const ratio = 21.4;

    expect(resolve('common.modifier_currency', { value }, { currency: { currency: 'USD', ratio: 1 } })).toBe(new Intl.NumberFormat(defaultLocale, { style: 'currency', currency: 'USD' }).format(value));
    expect(resolve('common.modifier_currency', { value }, { currency: { currency: 'CZK', ratio } })).toBe(new Intl.NumberFormat(defaultLocale, { style: 'currency', currency: 'CZK' }).format(value * ratio));
  });
  it('`currency` defaults work', () => {
    const value = 10;
    const ratio = 21.4;
    const resolveUsd = resolverFor<{ value?: number }>(defaultLocale, createParser({ modifierDefaults: { currency: { currency: 'USD', ratio: 1 } } }));
    const resolveCzk = resolverFor<{ value?: number }>(defaultLocale, createParser({ modifierDefaults: { currency: { currency: 'CZK', ratio } } }));

    expect(resolveUsd('common.modifier_currency', { value })).toBe(new Intl.NumberFormat(defaultLocale, { style: 'currency', currency: 'USD' }).format(value));
    expect(resolveCzk('common.modifier_currency', { value })).toBe(new Intl.NumberFormat(defaultLocale, { style: 'currency', currency: 'CZK' }).format(value * ratio));
  });
  it('custom modifier works', () => {
    const resolve = resolverFor<{ data?: any }>(defaultLocale, createParser({
      customModifiers: {
        test: ({ value }) => value,
      },
    }));

    expect(resolve('common.modifier_custom', { data: 'TEST_STRING' })).toBe('TEST_STRING');
  });
  it('a modifier the parser does not know resolves to the fallback chain and reports it', () => {
    const reports: Report[] = [];
    const { resolve } = createParser({ onReport: (report) => { reports.push(report); } });

    expect(resolve('{{value:plural; 1:one; default:many}}', { payload: { value: 1 }, key: 'common.plural' })).toBe('many');
    expect(resolve('{{value:plural; 1:one}}', { payload: { value: 1 } })).toBe('');
    expect(resolve('{{value:GT; 1:ONE; default:FALLBACK}}', { payload: { value: 1 } })).toBe('FALLBACK');
    expect(resolve('{{value:x-icon; ok:CHECK; default:FALLBACK}}', { payload: { value: 'ok' } })).toBe('FALLBACK');
    expect(resolve('{{value:plural; 1:one; default:FALLBACK}}', { payload: {} })).toBe('FALLBACK');

    expect(reports).toHaveLength(5);
    expect(reports[0]).toEqual({
      code: 'unknown-modifier',
      message: 'A placeholder named a modifier this parser does not know.',
      key: 'common.plural',
      text: '{{value:plural; 1:one; default:many}}',
    });
  });
  it('a modifier the caller registers is one the parser knows', () => {
    const reports: Report[] = [];
    const { resolve } = createParser({
      customModifiers: { plural: ({ value }) => (`${value}` === '1' ? 'one' : 'many') },
      onReport: (report) => { reports.push(report); },
    });

    expect(resolve('{{value:plural}}', { payload: { value: 1 } })).toBe('one');
    expect(resolve('{{value:plural}}', { payload: { value: 7 } })).toBe('many');

    expect(reports).toHaveLength(0);
  });
  it('a modifier reads a payload default at the type the payload gave it', () => {
    const seen: unknown[] = [];
    const { resolve } = createParser({ customModifiers: { test: ({ defaultValue }) => { seen.push(defaultValue); return 'DONE'; } } });

    expect(resolve('{{value:test}}', { payload: { value: 'V', default: 0 } })).toBe('DONE');
    expect(resolve('{{value:test}}', { payload: { value: 'V', default: false } })).toBe('DONE');
    expect(resolve('{{value:test}}', { payload: { value: 'V', default: null } })).toBe('DONE');
    expect(resolve('{{value:test}}', { payload: { value: 'V', default: [1, 2] } })).toBe('DONE');
    expect(resolve('{{value:test; default:INLINE}}', { payload: { value: 'V' } })).toBe('DONE');

    expect(seen).toEqual([0, false, null, [1, 2], 'INLINE']);
  });
  it('a payload default a modifier cannot turn into text still fails soft', () => {
    const { resolve } = defaultParser;
    const raising = { toString: () => { throw new Error('NO TEXT'); } };

    expect(resolve('{{value; 1:ONE}}', { payload: { value: 2, default: raising } })).toBe('');
    expect(resolve('{{value}}', { payload: { default: raising } })).toBe('');
  });
  it('a modifier that raises resolves to the fallback chain', () => {
    const throwing = createParser({ customModifiers: { test: () => { throw new Error('MODIFIER FAILURE'); } } });
    const resolveThrowing = resolverFor<{ data?: any }>(defaultLocale, throwing);
    const resolve = resolverFor<{ value?: any }>(defaultLocale);

    expect(resolveThrowing('common.modifier_custom_default', { data: 'TEST_STRING' })).toBe('FALLBACK');
    expect(resolveThrowing('common.modifier_custom', { data: 'TEST_STRING' })).toBe('');

    expect(resolve('common.modifier_currency_default', { value: 10 })).toBe('FALLBACK');

    const { resolve: resolveRaw } = defaultParser;

    expect(resolveRaw(message(defaultLocale, 'common.modifier_number_default'), { payload: { value: 10 }, locale: 'not a locale' })).toBe('FALLBACK');
  });
  it('a formatting modifier resolves to the empty string with no locale', () => {
    const { resolve } = defaultParser;

    for (const modifier of ['number', 'date', 'ago', 'currency']) {
      expect(resolve(`{{value:${modifier}; default:FALLBACK;}}`, { payload: { value: 10 } })).toBe('');
    }

    expect(resolve(message(defaultLocale, 'common.modifier_number_default'), { payload: { value: 10 } })).toBe('');
  });
  it('a value that cannot become text resolves to the fallback chain', () => {
    const { resolve } = defaultParser;
    const opaque = Object.create(null);
    const raising = { toString: () => { throw new Error('TO STRING FAILURE'); } };

    expect(resolve(opaque)).toBe('');
    expect(resolve(raising)).toBe('');

    expect(resolve('{{value}}', { payload: { value: opaque } })).toBe('');
    expect(resolve('{{value}}', { payload: { value: raising, default: 'FALLBACK' } })).toBe('FALLBACK');
    expect(resolve('{{value}}', { payload: { value: 'TEST_STRING', default: opaque } })).toBe('TEST_STRING');

    const { resolve: resolveThrowing } = createParser({ customModifiers: { test: () => { throw new Error('MODIFIER FAILURE'); } } });

    expect(resolveThrowing('{{value:test}}', { payload: { value: 'TEST_STRING', default: opaque } })).toBe('');

    const { resolve: resolveOpaque } = createParser({ customModifiers: { test: () => opaque } });

    expect(resolveOpaque('{{value:test; default:FALLBACK}}', { payload: { value: 'TEST_STRING' } })).toBe('FALLBACK');
  });
  it('a custom modifier map the host will not describe reads as no custom modifiers', () => {
    const revocable = Proxy.revocable({}, {});

    revocable.revoke();

    const hostile = [
      revocable.proxy,
      new Proxy({}, { ownKeys() { throw new Error('CUSTOM MODIFIERS FAILURE'); } }),
      Object.defineProperty({}, 'test', { enumerable: true, get() { throw new Error('CUSTOM MODIFIERS FAILURE'); } }),
    ];

    for (const customModifiers of hostile) {
      const { resolve } = createParser({ customModifiers: customModifiers as NonNullable<Parser.Options>['customModifiers'] });

      expect(resolve('{{value}}', { payload: { value: 'TEST_STRING' } })).toBe('TEST_STRING');
      expect(resolve('{{value:test}}', { payload: { value: 'TEST_STRING', default: 'FALLBACK' } })).toBe('FALLBACK');
    }
  });
  it('a custom modifier set to `undefined` leaves the modifier beneath it standing', () => {
    const { resolve } = createParser({ customModifiers: { eq: undefined } as unknown as NonNullable<Parser.Options>['customModifiers'] });

    expect(resolve('{{value; TEST_STRING:HIT}}', { payload: { value: 'TEST_STRING' } })).toBe('HIT');
  });
  it('a payload member that raises when read resolves to the fallback chain', () => {
    const { resolve } = defaultParser;
    const raise = () => { throw new Error('PAYLOAD MEMBER FAILURE'); };

    expect(resolve('{{value}}', { payload: { get value() { return raise(); }, default: 'FALLBACK' } })).toBe('FALLBACK');
    expect(resolve('{{value}}', { payload: { get default() { return raise(); } } })).toBe('');
    expect(resolve(undefined, { payload: { get default() { return raise(); } }, key: 'KEY' })).toBe('KEY');
  });
  it('a formatting modifier that cannot format its input resolves to the fallback chain', () => {
    const resolve = resolverFor<{ value?: any, default?: string }>(defaultLocale);

    expect(resolve('common.modifier_number_default', { value: 'not a number' })).toBe('FALLBACK');
    expect(resolve('common.modifier_currency_default', { value: 'not a number' }, { currency: { currency: 'USD', ratio: 1 } })).toBe('FALLBACK');
    expect(resolve('common.modifier_date', { value: 'not a number', default: 'FALLBACK' })).toBe('FALLBACK');
    expect(resolve('common.modifier_ago', { value: 'not a number', default: 'FALLBACK' })).toBe('FALLBACK');
    expect(resolve('common.modifier_date', { value: 'not a number' })).toBe('');
  });
  it('a formatting modifier formats zero rather than falling back', () => {
    const resolve = resolverFor<{ value?: any }>(defaultLocale);

    expect(resolve('common.modifier_number_default', { value: 0 })).toBe(new Intl.NumberFormat(defaultLocale, { maximumFractionDigits: 2 }).format(0));
    expect(resolve('common.modifier_currency_default', { value: 0 }, { currency: { currency: 'USD', ratio: 1 } })).toBe(new Intl.NumberFormat(defaultLocale, { style: 'currency', currency: 'USD' }).format(0));
  });
  it('modifiers containing escaped values work', () => {
    const resolve = resolverFor<{ 'va:lue'?: any }>(defaultLocale);

    expect(resolve('common.modifier_escaped', { 'va:lue': 'option:1' })).toBe('VA;{{LUE}}:1');
    expect(resolve('common.modifier_escaped', { 'va:lue': 'option:2' })).toBe('VA;{{LUE}}:2');
    expect(resolve('common.modifier_escaped')).toBe('DEFAULT {{VALUE}};');
  });
  it('single character default value works', () => {
    const resolve = resolverFor<{ age?: number, value?: any }>(defaultLocale);

    expect(resolve('common.modifier_default_single_char', { age: 7 })).toBe('as a 7-year-old');
    expect(resolve('common.modifier_default_single_char', { age: 18 })).toBe('as an 18-year-old');
    expect(resolve('common.placeholder_default_single_char')).toBe('VALUES: a, a, a, a');
    expect(resolve('common.placeholder_default_single_char', { value: 'TEST_VALUE' })).toBe('VALUES: TEST_VALUE, TEST_VALUE, TEST_VALUE, TEST_VALUE');
  });
  it('escaped semicolons in default values work', () => {
    const resolve = resolverFor<{ value?: any }>(defaultLocale);

    expect(resolve('common.placeholder_default_escaped')).toBe('VALUES: ;SEMI, SEMI;, ;');
    expect(resolve('common.placeholder_default_escaped', { value: 'TEST_VALUE' })).toBe('VALUES: TEST_VALUE, TEST_VALUE, TEST_VALUE');
  });
  it('short keys work', () => {
    const resolve = resolverFor<{ n?: any, nn?: any }>(defaultLocale);

    expect(resolve('common.placeholder_short_key', { n: 'TEST_VALUE', nn: 'TEST_VALUE' })).toBe('VALUES: TEST_VALUE, TEST_VALUE, TEST_VALUE, TEST_VALUE, TEST_VALUE');
    expect(resolve('common.modifier_short_key', { n: 1, nn: 1 })).toBe('VALUES: VALUE1, VALUE1, DEFAULT VALUE');
    expect(resolve('common.modifier_short_key', { n: 15, nn: 10 })).toBe('VALUES: DEFAULT VALUE, VALUE2, VALUE2');
    expect(resolve('common.modifier_short_key')).toBe('VALUES: DEFAULT VALUE, DEFAULT VALUE, DEFAULT VALUE');
  });
  it('short option segments work', () => {
    const resolve = resolverFor<{ value?: any }>(defaultLocale);

    expect(resolve('common.modifier_short_option')).toBe('VALUES: DEF, DEF, z');
    expect(resolve('common.modifier_short_option', { value: 'x' })).toBe('VALUES: , DEF, z');
    expect(resolve('common.modifier_short_option', { value: 5 })).toBe('VALUES: FIVE, , z');
    expect(resolve('common.modifier_short_option', { value: 2 })).toBe('VALUES: DEF, , z');
  });
  it('an option that ends at its colon declares an empty value', () => {
    const { resolve } = defaultParser;

    expect(resolve('{{v; 1:; 2:TWO}}', { payload: { v: 1 } })).toBe('');
    expect(resolve('{{v; 1: ; 2:TWO}}', { payload: { v: 1 } })).toBe('');
    expect(resolve('{{v; 1:; default:D}}', { payload: { v: 1 } })).toBe('');
    expect(resolve('{{v:gt; 1:; default:D}}', { payload: { v: 7 } })).toBe('');
    expect(resolve('{{v; 2:; default:D}}', { payload: { v: 1 } })).toBe('D');
  });
  it('an option that names no value at all stands for itself', () => {
    const { resolve } = defaultParser;

    expect(resolve('{{v; 1}}', { payload: { v: 1 } })).toBe('1');
    expect(resolve('{{v; 2}}', { payload: { v: 1 } })).toBe('');
    expect(resolve('{{v; 2; default:D}}', { payload: { v: 1 } })).toBe('D');
    expect(resolve('{{v:ne; z; default:D}}', { payload: { v: 'a' } })).toBe('z');
  });
  it('an option value and an inline default are trimmed on both sides', () => {
    const { resolve } = defaultParser;

    expect(resolve('{{v; 1:ONE   ; 2:TWO}}', { payload: { v: 1 } })).toBe('ONE');
    expect(resolve('{{v; 1:   ONE}}', { payload: { v: 1 } })).toBe('ONE');
    expect(resolve('{{v; 1 : ONE ; default : D }}', { payload: { v: 1 } })).toBe('ONE');
    expect(resolve('{{v;  default : D }}', { payload: {} })).toBe('D');
    expect(resolve('{{v; 1:   ; default:D}}', { payload: { v: 1 } })).toBe('');
  });
  it('an option value keeps every colon after the first', () => {
    const { resolve } = defaultParser;

    expect(resolve('{{v; 1:10:30; 2:11:30}}', { payload: { v: 1 } })).toBe('10:30');
    expect(resolve('{{v; 1:10:30; 2:11:30}}', { payload: { v: 2 } })).toBe('11:30');
    expect(resolve('{{v; 1:10\\:30}}', { payload: { v: 1 } })).toBe('10:30');
    expect(resolve('{{v; 1:https://example.com/a:b; default:D}}', { payload: { v: 1 } })).toBe('https://example.com/a:b');
  });
  it('a backslash escapes every character the syntax reserves', () => {
    const { resolve } = defaultParser;

    expect(resolve('a\\\\b')).toBe('a\\b');
    expect(resolve('a\\ b')).toBe('a b');
    expect(resolve('a\\;b')).toBe('a;b');
    expect(resolve('a\\{\\{b')).toBe('a{{b');
  });
  it('a backslash before anything else is text', () => {
    const { resolve } = defaultParser;

    expect(resolve('C:\\Users\\name')).toBe('C:\\Users\\name');
    expect(resolve('\\d+')).toBe('\\d+');
  });
  it('escaped whitespace is text, not padding', () => {
    const { resolve } = defaultParser;

    expect(resolve('{{v; 1:ONE\\ }}', { payload: { v: 1 } })).toBe('ONE ');
    expect(resolve('{{v; 1:\\ ONE}}', { payload: { v: 1 } })).toBe(' ONE');
    expect(resolve('{{v\\ x; 1:ONE}}', { payload: { 'v x': 1 } })).toBe('ONE');
    expect(resolve('{{v; \\ :X; default:D}}', { payload: { v: ' ' } })).toBe('X');
    expect(resolve('{{v; \\ :X; default:D}}', { payload: { v: 'x' } })).toBe('D');
  });
  it('an escaped backslash does not escape what follows it', () => {
    const { resolve } = defaultParser;

    expect(resolve('{{v; a\\\\:b:X}}', { payload: { v: 'a\\' } })).toBe('b:X');
    expect(resolve('{{v; a\\\\:b:X}}', { payload: { v: 'a\\:b' } })).toBe('');
    expect(resolve('{{v; a\\\\:b:X; default:D}}', { payload: { v: 'z' } })).toBe('D');
  });
  it('an inline default keeps every colon after the first', () => {
    const { resolve } = defaultParser;

    expect(resolve('{{v; default::x}}')).toBe(':x');
    expect(resolve('{{v; default:10:30}}')).toBe('10:30');
    expect(resolve('{{v; default:https://example.com/a:b}}')).toBe('https://example.com/a:b');
  });
  it('a payload value is unescaped by the same rule as the message', () => {
    const { resolve } = defaultParser;

    expect(resolve('{{v}}', { payload: { v: 'a\\\\b' } })).toBe('a\\b');
    expect(resolve('{{v}}', { payload: { v: 'a\\ b' } })).toBe('a b');
    expect(resolve('{{v}}', { payload: { v: '\\\\server\\share' } })).toBe('\\server\\share');
    expect(resolve('{{v}}', { payload: { v: 'C:\\Users\\name' } })).toBe('C:\\Users\\name');
  });
  it('keys starting with an escaped semicolon work', () => {
    const resolve = resolverFor<{ ';value'?: any }>(defaultLocale);

    expect(resolve('common.placeholder_escaped_leading', { ';value': 'TEST_VALUE' })).toBe('VALUES: TEST_VALUE, DEFAULT VALUE');
    expect(resolve('common.placeholder_escaped_leading', { ';value': 1 })).toBe('VALUES: 1, VALUE1');
    expect(resolve('common.placeholder_escaped_leading')).toBe('VALUES: , DEFAULT VALUE');
  });
  it('unparsable placeholders do not resolve a payload key', () => {
    const resolve = resolverFor<{ [key: string]: any }>(defaultLocale);

    expect(resolve('common.placeholder_unparsable', { null: 'LEAKED_VALUE' })).toBe('VALUES: , , ');
    expect(resolve('common.placeholder_unparsable', { undefined: 'LEAKED_VALUE' })).toBe('VALUES: , , ');
    expect(resolve('common.placeholder_unparsable', { '': 'LEAKED_VALUE' })).toBe('VALUES: , , ');
    expect(resolve('common.placeholder_unparsable', { null: 'LEAKED_VALUE', default: 'DEFAULT VALUE' })).toBe('VALUES: DEFAULT VALUE, DEFAULT VALUE, DEFAULT VALUE');
  });
  it('inherited payload members are not resolved', () => {
    const resolve = resolverFor<{ [key: string]: any }>(defaultLocale);

    expect(resolve('common.placeholder_inherited')).toBe('VALUES: , , , INLINE DEFAULT');
    expect(resolve('common.placeholder_inherited', { default: 'DEFAULT VALUE' })).toBe('VALUES: DEFAULT VALUE, DEFAULT VALUE, DEFAULT VALUE, DEFAULT VALUE');
    expect(resolve('common.placeholder_inherited', { constructor: 'OWN VALUE' })).toBe('VALUES: OWN VALUE, , , INLINE DEFAULT');

    const inherited = Object.create({ default: 'INHERITED DEFAULT' });

    expect(resolve('common.placeholder', inherited)).toBe('VALUES: , , , ');
    expect(resolve('common.undefined', inherited)).toBe('common.undefined');
  });
  it('self-referential payload values do not overflow', () => {
    const resolve = resolverFor<{ value?: any, first?: string, second?: string }>(defaultLocale);

    expect(resolve('common.placeholder', { value: '{{value}}' })).toBe('VALUES: {{value}}, {{value}}, {{value}}, {{value}}');
    expect(resolve('common.placeholder', { value: '{{first}}', first: '{{second}}', second: 'TEST_VALUE' })).toBe('VALUES: TEST_VALUE, TEST_VALUE, TEST_VALUE, TEST_VALUE');
  });
  it('reaching the interpolation cap reports a bounded excerpt', () => {
    const reports: Report[] = [];
    const resolve = resolverFor<{ [key: string]: any }>(defaultLocale, createParser({ onReport: (report) => { reports.push(report); } }));

    const chain = (length: number) => Array.from({ length }, (_, i) => [`v${i + 1}`, i + 1 === length ? 'END' : `{{v${i + 2}}}`])
      .reduce((acc, [key, value]) => ({ ...acc, [key]: value }), {} as Record<string, any>);

    expect(resolve('common.placeholder_chain', chain(10))).toBe('END');
    expect(resolve('common.placeholder_chain', chain(11))).toBe('{{v11}}');

    expect(reports).toHaveLength(1);
    expect(reports[0]).toEqual({
      code: 'pass-limit',
      message: 'Interpolation stopped after 10 passes. A payload value probably references its own placeholder.',
      key: 'common.placeholder_chain',
      limit: 10,
      text: '{{v11}}',
    });
  });
  it('a report never carries a line terminator out of the payload', () => {
    const reports: Report[] = [];
    const resolve = resolverFor<{ v1?: string }>(defaultLocale, createParser({ onReport: (report) => { reports.push(report); } }));

    for (const terminator of ['\n', '\r', '\u2028', '\u2029']) {
      expect(resolve('common.placeholder_chain', { v1: `{{v1}}${terminator}[i18n]: FORGED${'x'.repeat(1000)}` })).toContain('[i18n]: FORGED');
    }

    expect(reports).toHaveLength(4);

    for (const report of reports) {
      expect(report.text.length).toBeLessThan(300);
      expect(report.text).not.toMatch(/[\n\r\u2028\u2029]/);
      expect(report.message).not.toContain('FORGED');
    }
  });
  it('exceeding the output budget stops interpolation and reports it', () => {
    const reports: Report[] = [];
    const resolve = resolverFor<{ v1?: string }>(defaultLocale, createParser({ onReport: (report) => { reports.push(report); } }));

    const output = resolve('common.placeholder_chain', { v1: `${'{{v1}}'.repeat(4)}${'x'.repeat(64)}` });

    expect(output.length).toBeLessThanOrEqual(100000);
    expect(output.length).toBe(27968);

    expect(reports).toHaveLength(1);
    expect(reports[0]).toMatchObject({ code: 'output-limit', limit: 100000, key: 'common.placeholder_chain' });
    expect(reports[0].text.length).toBeLessThan(300);
  });
  it('a pass is bounded as it is built, so it cannot outgrow what a string can hold', () => {
    const reports: Report[] = [];
    const { resolve } = createParser({ onReport: (report) => { reports.push(report); } });

    const multiplied = '{{b}}'.repeat(10400);
    const output = resolve('{{a}}', { payload: { a: multiplied, b: 'x'.repeat(52000) }, key: 'common.placeholder_chain' });

    expect(output).toBe(multiplied);

    expect(reports).toHaveLength(1);
    expect(reports[0]).toMatchObject({ code: 'output-limit', limit: 100000, key: 'common.placeholder_chain' });
  });
  it('without `onReport` a parser reports nowhere and still fails soft', () => {
    const { warn } = console;
    const warnings: unknown[] = [];

    console.warn = (...args: unknown[]) => { warnings.push(args); };

    try {
      const resolve = resolverFor<{ v1?: string }>(defaultLocale);

      expect(resolve('common.placeholder_chain', { v1: '{{v1}}' })).toBe('{{v1}}');
    } finally {
      console.warn = warn;
    }

    expect(warnings).toHaveLength(0);
  });
  const timePerOp = (run: () => void) => {
    run();

    return Math.min(...Array.from({ length: 3 }, () => {
      const start = performance.now();
      let iterations = 0;
      let elapsed = 0;

      do {
        run();
        iterations += 1;
        elapsed = performance.now() - start;
      } while (elapsed < 25);

      return elapsed / iterations;
    }));
  };

  // Quadrupling the input puts the square root of the per-op ratio near 2 when
  // cost is linear, 4 when quadratic and 8 when cubic; asserting < 3 passes a
  // linear scan with headroom and fails any polynomial backtracking.
  const growthWhenInputQuadruples = (runAt: (size: number) => () => void, size: number) => Math.sqrt(timePerOp(runAt(size * 4)) / timePerOp(runAt(size)));
  it('interpolating a padded placeholder costs linear time', () => {
    const resolve = resolverFor<{ value?: any }>(defaultLocale);

    const runAt = (size: number) => {
      const payload = { value: `{{${' '.repeat(size)}}}` };

      return () => { resolve('common.placeholder', payload); };
    };

    expect(growthWhenInputQuadruples(runAt, 240)).toBeLessThan(3);
  }, 30000);
  it('interpolating a placeholder key padded with inner whitespace costs linear time', () => {
    const resolve = resolverFor<{ value?: any }>(defaultLocale);

    const runAt = (size: number) => {
      const payload = { value: `{{a${' '.repeat(size)}b}}` };

      return () => { resolve('common.placeholder', payload); };
    };

    expect(growthWhenInputQuadruples(runAt, 500)).toBeLessThan(3);
  }, 30000);
  it('scanning an unclosed trailing placeholder costs linear time', () => {
    const resolve = resolverFor<{ value?: any, a?: string }>(defaultLocale);

    const runAt = (size: number) => {
      const payload = { value: `{{a}}{{${' \n'.repeat(size / 2)}`, a: 'A' };

      return () => { resolve('common.placeholder', payload); };
    };

    expect(growthWhenInputQuadruples(runAt, 500)).toBeLessThan(3);
  }, 60000);
  it('collecting a long modifier options list costs linear time', () => {
    const resolve = resolverFor<{ value?: any, a?: string }>(defaultLocale);

    const runAt = (size: number) => {
      const payload = { value: `{{${'a;'.repeat(size / 2)}}}`, a: 'A' };

      return () => { resolve('common.placeholder', payload); };
    };

    expect(growthWhenInputQuadruples(runAt, 500)).toBeLessThan(3);
  }, 30000);
  it('splitting a long modifier option costs linear time', () => {
    const resolve = resolverFor<{ value?: any, a?: string }>(defaultLocale);

    const runAt = (size: number) => {
      const payload = { value: `{{a; ${'x'.repeat(size)}:v}}`, a: 'A' };

      return () => { resolve('common.placeholder', payload); };
    };

    expect(growthWhenInputQuadruples(runAt, 500)).toBeLessThan(3);
  }, 30000);
});
