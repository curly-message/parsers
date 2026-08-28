import { describe, expect, it } from 'vitest';
import { createParser, Parser, Report } from '../../src';
import { LINE_TERM } from '../../src/utils';
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

// Two values no conversion can describe: `JSON.stringify` raises on the
// circular one, and `String` on the class instance, which its prototype keeps
// off the JSON path.
const circular: Record<string, unknown> = {};

circular.self = circular;

class Opaque {
  toString(): string {
    throw new Error('NO TEXT');
  }
}

// The format's whitespace class (SPEC.md section 6). Every member is written as
// an escape: a literal invisible code point is invisible to review and to a
// diff.
const WHITESPACE = [
  '\u0009', '\u000a', '\u000b', '\u000c', '\u000d', '\u0020', '\u00a0', '\u1680',
  '\u2000', '\u2001', '\u2002', '\u2003', '\u2004', '\u2005', '\u2006', '\u2007',
  '\u2008', '\u2009', '\u200a', '\u2028', '\u2029', '\u202f', '\u205f', '\u3000',
  '\ufeff',
];

// The four the class shares with `line-term`, which a placeholder holds in no
// position (section 6, note 1), so only the rest reach the trimming rules.
const LINE_TERMINATORS = ['\u000a', '\u000d', '\u2028', '\u2029'];

const PLACEHOLDER_WHITESPACE = WHITESPACE.filter((character) => !LINE_TERMINATORS.includes(character));

// A code point as the label a specification writes it under, so a failure
// names the character instead of printing it invisibly.
const codePoint = (character: string) => `U+${character.charCodeAt(0).toString(16).toUpperCase().padStart(4, '0')}`;

// Code points some other notion of whitespace reaches for and this class does
// not: U+0085, which Unicode's White_Space property holds; U+001C-U+001F, which
// Python's `str.isspace` holds; U+180E, which White_Space held until UCD 6.3.0;
// and six more that only look blank.
const NOT_WHITESPACE = [
  '\u0085', '\u001c', '\u001d', '\u001e', '\u001f', '\u180e', '\u200b', '\u2060',
  '\u061c', '\u00ad', '\u3164', '\u2800',
];

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
  it('a payload value reaches a placeholder as text', () => {
    class Amount {
      toString(): string {
        return '10 USD';
      }
    }

    const { resolve } = defaultParser;
    const stamp = Date.parse('2024-03-05T10:00:00.000Z');

    expect(resolve('{{v}}', { payload: { v: { a: 1, b: 'x' } } })).toBe('{"a":1,"b":"x"}');
    expect(resolve('{{v}}', { payload: { v: Object.assign(Object.create(null), { a: 1 }) } })).toBe('{"a":1}');
    expect(resolve('{{v}}', { payload: { v: [1, 2] } })).toBe('[1,2]');
    expect(resolve('{{v}}', { payload: { v: ['a', { b: 2 }] } })).toBe('["a",{"b":2}]');

    expect(resolve('{{v}}', { payload: { v: new Date(stamp) } })).toBe(String(new Date(stamp)));
    expect(resolve('{{v}}', { payload: { v: /ab+c/gi } })).toBe('/ab+c/gi');
    expect(resolve('{{v}}', { payload: { v: new Map([['a', 1]]) } })).toBe('[object Map]');
    expect(resolve('{{v}}', { payload: { v: new Set([1, 2]) } })).toBe('[object Set]');
    expect(resolve('{{v}}', { payload: { v: new Amount() } })).toBe('10 USD');

    expect(resolve('{{v}}', { payload: { v: null } })).toBe('null');
    expect(resolve('{{v}}', { payload: { v: NaN } })).toBe('NaN');
    expect(resolve('{{v}}', { payload: { v: {} } })).toBe('{}');
  });
  it('a payload entry owning only wrapper keys is a wrapper', () => {
    const { resolve } = defaultParser;

    expect(resolve('{{v}}', { payload: { v: { value: 1 } } })).toBe('1');
    expect(resolve('{{v}}', { payload: { v: { value: 1, default: 'D' } } })).toBe('1');
    expect(resolve('{{v}}', { payload: { v: { default: 'D' } } })).toBe('D');
    expect(resolve('{{v:number; default:INLINE}}', { payload: { v: { props: { number: {} } } }, locale: defaultLocale })).toBe('INLINE');
  });
  it('a payload entry owning anything else is a value', () => {
    const { resolve } = defaultParser;

    expect(resolve('{{v}}', { payload: { v: { value: 1, unit: 'kg' } } })).toBe('{"value":1,"unit":"kg"}');
    expect(resolve('{{v}}', { payload: { v: { valu: 'X' } } })).toBe('{"valu":"X"}');
    expect(resolve('{{v}}', { payload: { v: {} } })).toBe('{}');
    expect(resolve('{{v}}', { payload: { v: ['value'] } })).toBe('["value"]');
  });
  it('a payload entry is unwrapped exactly once', () => {
    const { resolve } = defaultParser;

    expect(resolve('{{v}}', { payload: { v: { value: { a: 1 } } } })).toBe('{"a":1}');
    expect(resolve('{{v}}', { payload: { v: { value: { value: 2 } } } })).toBe('{"value":2}');
    expect(resolve('{{v}}', { payload: { default: { value: 'X' } } })).toBe('{"value":"X"}');
  });
  it('a wrapper carries a value unless that value is `undefined`', () => {
    const { resolve } = defaultParser;

    expect(resolve('{{v}}', { payload: { v: { value: undefined, default: 'D' } } })).toBe('D');
    expect(resolve('{{v}}', { payload: { v: { value: null, default: 'D' } } })).toBe('null');
    expect(resolve('{{v}}', { payload: { v: { value: 0, default: 'D' } } })).toBe('0');
    expect(resolve('{{v}}', { payload: { v: { value: false, default: 'D' } } })).toBe('false');
    expect(resolve('{{v}}', { payload: { v: { value: '', default: 'D' } } })).toBe('');
    expect(resolve('{{v}}', { payload: { v: { value: NaN, default: 'D' } } })).toBe('NaN');
  });
  it('the payload\'s root `default` is a value, never a wrapper', () => {
    const { resolve } = defaultParser;

    expect(resolve('{{default}}', { payload: { default: { value: 'X' } } })).toBe('{"value":"X"}');
    expect(resolve('{{v}}', { payload: { default: { value: 'X' } } })).toBe('{"value":"X"}');
  });
  it('a wrapper default outranks the payload default, which outranks the inline one', () => {
    const { resolve } = defaultParser;

    expect(resolve('{{v; default:INLINE}}', { payload: { v: { default: 'WRAPPER' }, default: 'PAYLOAD' } })).toBe('WRAPPER');
    expect(resolve('{{v; default:INLINE}}', { payload: { v: { value: undefined }, default: 'PAYLOAD' } })).toBe('PAYLOAD');
    expect(resolve('{{v; default:INLINE}}', { payload: { default: 'PAYLOAD' } })).toBe('PAYLOAD');
    expect(resolve('{{v; default:INLINE}}', { payload: {} })).toBe('INLINE');
    expect(resolve('{{v}}', { payload: {} })).toBe('');
  });
  it('a default link that cannot become text is skipped', () => {
    const { resolve } = defaultParser;

    expect(resolve('{{v; default:INLINE}}', { payload: { v: { default: circular }, default: 'PAYLOAD' } })).toBe('PAYLOAD');
    expect(resolve('{{v; default:INLINE}}', { payload: { v: { default: circular }, default: new Opaque() } })).toBe('INLINE');
    expect(resolve('{{v}}', { payload: { v: { default: circular }, default: new Opaque() } })).toBe('');
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
    expect(resolve('common.modifier_ne')).toBe('DEFAULT VALUE');
  });
  it('`eq` and `ne` compare an option key against a value case-insensitively', () => {
    const { resolve } = defaultParser;

    expect(resolve('{{v:eq; YES:A; default:D}}', { payload: { v: 'yes' } })).toBe('A');
    expect(resolve('{{v:eq; yes:A; default:D}}', { payload: { v: 'YES' } })).toBe('A');
    expect(resolve('{{v:eq; yEs:A; default:D}}', { payload: { v: 'YeS' } })).toBe('A');

    expect(resolve('{{v:ne; YES:A; NO:B; default:D}}', { payload: { v: 'yes' } })).toBe('B');
    expect(resolve('{{v:ne; yes:A; default:D}}', { payload: { v: 'YES' } })).toBe('D');
  });
  it('an absent value takes the fallback chain under `ne` like every other modifier', () => {
    const { resolve } = defaultParser;

    expect(resolve('{{v:ne; 10:V2; default:D}}')).toBe('D');
    expect(resolve('{{v:ne; a:A; b:B}}')).toBe('');
    expect(resolve('{{v:ne; 10:V2; default:D}}', { payload: { v: 10 } })).toBe('D');
    expect(resolve('{{v:ne; 10:V2; default:D}}', { payload: { v: 5 } })).toBe('V2');
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
  it('a numeric comparison orders its options and skips a key that is not numeric', () => {
    const { resolve } = defaultParser;

    expect(resolve('{{v:lt; 10:TEN; 5:FIVE; default:D}}', { payload: { v: 1 } })).toBe('FIVE');
    expect(resolve('{{v:gt; 5:FIVE; 10:TEN; default:D}}', { payload: { v: 20 } })).toBe('TEN');
    expect(resolve('{{v:lt; 10:TEN; abc:ABC; 5:FIVE; default:D}}', { payload: { v: 1 } })).toBe('FIVE');
    expect(resolve('{{v:gt; 5:FIVE; abc:ABC; 10:TEN; default:D}}', { payload: { v: 20 } })).toBe('TEN');
    expect(resolve('{{v:lt; abc:ABC; default:D}}', { payload: { v: 1 } })).toBe('D');
    expect(resolve('{{v:gte; abc:ABC; 5:FIVE; default:D}}', { payload: { v: 7 } })).toBe('FIVE');
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
  it('the two-digit maximum widens to reach a minimum a layer names', () => {
    const { resolve } = createParser({ modifierDefaults: { number: { minimumFractionDigits: 4 } } });

    expect(resolve('{{v:number; default:D}}', { payload: { v: 1.5 }, locale: defaultLocale })).toBe('1.5000');
    expect(resolve('{{v:number; default:D}}', { payload: { v: 1.5 }, props: { number: { minimumFractionDigits: 6 } }, locale: defaultLocale })).toBe('1.500000');
    // A minimum under the default leaves the default in place, and a named
    // maximum is used as named.
    expect(resolve('{{v:number}}', { payload: { v: { value: 1.23456, props: { number: { minimumFractionDigits: 1 } } } }, locale: defaultLocale })).toBe('1.23');
    expect(resolve('{{v:number}}', { payload: { v: { value: 1.23456, props: { number: { maximumFractionDigits: 5 } } } }, locale: defaultLocale })).toBe('1.23456');
  });
  it('a zero in `modifierDefaults` is a value, not an absence', () => {
    const { resolve } = createParser({ modifierDefaults: { number: { maximumFractionDigits: 0 }, currency: { ratio: 0, currency: 'EUR' } } });

    expect(resolve('{{v:number}}', { payload: { v: 1234.56 }, locale: defaultLocale })).toBe('1,235');
    expect(resolve('{{v:currency}}', { payload: { v: 1234.56 }, locale: defaultLocale })).toBe('€0.00');
  });
  it('`props` compose per property over the parser defaults and the call', () => {
    const { resolve } = createParser({ modifierDefaults: { number: { maximumFractionDigits: 4, useGrouping: false } } });
    const value = 1234.56789;

    expect(resolve('{{v:number}}', { payload: { v: value }, locale: defaultLocale })).toBe('1234.5679');
    expect(resolve('{{v:number}}', { payload: { v: value }, props: { number: { useGrouping: true } }, locale: defaultLocale })).toBe('1,234.5679');
    expect(resolve('{{v:number}}', { payload: { v: { value, props: { number: { maximumFractionDigits: 1 } } } }, locale: defaultLocale })).toBe('1234.6');
    expect(resolve('{{v:number}}', { payload: { v: { value, props: { number: { maximumFractionDigits: 1 } } } }, props: { number: { useGrouping: true } }, locale: defaultLocale })).toBe('1,234.6');
  });
  it('a wrapper leaves every prop it does not name alone', () => {
    const seen: unknown[] = [];
    const { resolve } = createParser({ customModifiers: { test: ({ props }) => { seen.push(props); return 'DONE'; } } });

    expect(resolve('{{v:test}}', { payload: { v: { value: 1, props: { number: { maximumFractionDigits: 1 } } } }, props: { number: { useGrouping: true }, date: { timeStyle: 'full' } } })).toBe('DONE');
    expect(resolve('{{v:test}}', { payload: { v: 1 }, props: { number: { useGrouping: true } } })).toBe('DONE');

    expect(seen).toEqual([
      { number: { useGrouping: true, maximumFractionDigits: 1 }, date: { timeStyle: 'full' } },
      { number: { useGrouping: true } },
    ]);
  });
  it('a wrapper prop set to `undefined` leaves the layer beneath it standing', () => {
    const { resolve } = createParser();
    const value = 1234.56789;

    expect(resolve('{{v:number}}', { payload: { v: { value, props: { number: undefined } } }, props: { number: { useGrouping: true } }, locale: defaultLocale })).toBe('1,234.57');
    expect(resolve('{{v:number}}', { payload: { v: { value, props: { number: { useGrouping: undefined } } } }, props: { number: { useGrouping: true, maximumFractionDigits: 3 } }, locale: defaultLocale })).toBe('1,234.568');
  });
  it('a wrapper prop set to `undefined` leaves `modifierDefaults` standing', () => {
    const { resolve } = createParser({ modifierDefaults: { number: { maximumFractionDigits: 4, useGrouping: false } } });
    const value = 1234.56789;

    expect(resolve('{{v:number}}', { payload: { v: { value, props: { number: undefined } } }, locale: defaultLocale })).toBe('1234.5679');
    expect(resolve('{{v:number}}', { payload: { v: { value, props: { number: { useGrouping: undefined } } } }, locale: defaultLocale })).toBe('1234.5679');
  });
  it('a call prop set to `undefined` leaves `modifierDefaults` standing', () => {
    const { resolve } = createParser({ modifierDefaults: { number: { maximumFractionDigits: 4, useGrouping: false } } });
    const value = 1234.56789;

    expect(resolve('{{v:number}}', { payload: { v: value }, props: { number: { useGrouping: undefined, maximumFractionDigits: undefined } }, locale: defaultLocale })).toBe('1234.5679');
  });
  it('a modifier receives a payload-supplied `props` copied one level down', () => {
    const seen: any[] = [];
    const { resolve } = createParser({ customModifiers: { test: ({ props }) => { seen.push(props); return 'DONE'; } } });
    const wrapperProps = { number: { maximumFractionDigits: 1 } };

    expect(resolve('{{v:test}}', { payload: { v: { value: 1, props: wrapperProps } } })).toBe('DONE');
    expect(resolve('{{v:test}}', { payload: { v: { value: 1, props: wrapperProps } }, props: { date: { timeStyle: 'full' } } })).toBe('DONE');

    seen.forEach((props) => {
      expect(props).not.toBe(wrapperProps);
      expect(props.number).not.toBe(wrapperProps.number);
    });
  });
  it('merging a wrapper\'s `props` cannot reach a prototype', () => {
    const { resolve } = createParser({ customModifiers: { test: ({ props }) => JSON.stringify(props) } });
    const polluting = JSON.parse('{"__proto__":{"polluted":true}}');

    expect(resolve('{{v:test}}', { payload: { v: { value: 1, props: polluting } }, props: { number: {} } })).toBe('{"number":{},"__proto__":{"polluted":true}}');
    expect(({} as any).polluted).toBe(undefined);
  });
  it('configuration is read as own properties, never through a prototype', () => {
    const proto = Object.prototype as any;
    const polluted = (name: string, value: any, read: () => string) => {
      proto[name] = value;

      try {
        return read();
      } finally {
        delete proto[name];
      }
    };
    const payload = { v: 1.23456789 };
    const seen: Report[] = [];

    expect(polluted('customModifiers', { number: () => 'HIJACKED' }, () => createParser({}).resolve('{{v:number}}', { payload, locale: defaultLocale }))).toBe('1.23');
    expect(polluted('modifierDefaults', { number: { maximumFractionDigits: 5 } }, () => createParser({}).resolve('{{v:number}}', { payload, locale: defaultLocale }))).toBe('1.23');
    expect(polluted('number', { maximumFractionDigits: 5 }, () => createParser({ modifierDefaults: {} }).resolve('{{v:number}}', { payload, props: {}, locale: defaultLocale }))).toBe('1.23');
    expect(polluted('onReport', (entry: Report) => seen.push(entry), () => createParser({}).resolve('{{v:nosuch}}', { payload, locale: defaultLocale }))).toBe('');
    expect(seen).toEqual([]);
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

    expect(resolve('common.modifier_ago', { value })).toBe(new Intl.RelativeTimeFormat(defaultLocale, { numeric: 'auto' }).format(-30, 'minute'));
    expect(resolveAlt('common.modifier_ago', { value })).toBe(new Intl.RelativeTimeFormat(altLocale, { numeric: 'auto' }).format(-30, 'minute'));
  });
  it('`ago` asks for the word a locale keeps, unless a caller asks for the count', () => {
    const resolve = resolverFor<{ value?: any }>(defaultLocale);
    const resolveAlt = resolverFor<{ value?: any }>(altLocale);
    const resolveAlways = resolverFor<{ value?: any }>(defaultLocale, createParser({ modifierDefaults: { ago: { numeric: 'always' } } }));
    const value = -1000 * 60 * 60 * 24;

    // A single day out is where the two spellings part. Every other fixture
    // here sits at a magnitude they agree on, so only this one measures which
    // of the two the modifier actually asks for.
    expect(resolve('common.modifier_ago', { value })).toBe(new Intl.RelativeTimeFormat(defaultLocale, { numeric: 'auto' }).format(-1, 'day'));
    expect(resolve('common.modifier_ago', { value })).not.toBe(new Intl.RelativeTimeFormat(defaultLocale, { numeric: 'always' }).format(-1, 'day'));
    expect(resolveAlt('common.modifier_ago', { value })).toBe(new Intl.RelativeTimeFormat(altLocale, { numeric: 'auto' }).format(-1, 'day'));

    expect(resolve('common.modifier_ago', { value }, { ago: { numeric: 'always' } })).toBe(new Intl.RelativeTimeFormat(defaultLocale, { numeric: 'always' }).format(-1, 'day'));
    expect(resolveAlways('common.modifier_ago', { value })).toBe(new Intl.RelativeTimeFormat(defaultLocale, { numeric: 'always' }).format(-1, 'day'));
  });
  it('`ago` props work', () => {
    const resolve = resolverFor<{ value?: any }>(defaultLocale);
    const value = -1000 * 60 * 60 * 24 * 7;

    expect(resolve('common.modifier_ago', { value }, { ago: { format: 'day' } })).toBe(new Intl.RelativeTimeFormat(defaultLocale, { numeric: 'auto' }).format(-7, 'day'));
    expect(resolve('common.modifier_ago', { value }, { ago: { format: 'week' } })).not.toBe(new Intl.RelativeTimeFormat(defaultLocale, { numeric: 'auto' }).format(-7, 'day'));
  });
  it('`ago` defaults work', () => {
    const resolveDays = resolverFor<{ value?: any }>(defaultLocale, createParser({ modifierDefaults: { ago: { format: 'days' } } }));
    const resolveWeek = resolverFor<{ value?: any }>(defaultLocale, createParser({ modifierDefaults: { ago: { format: 'week' } } }));
    const value = -1000 * 60 * 60 * 24 * 7;

    expect(resolveDays('common.modifier_ago', { value })).toBe(new Intl.RelativeTimeFormat(defaultLocale, { numeric: 'auto' }).format(-7, 'day'));
    expect(resolveWeek('common.modifier_ago', { value })).not.toBe(new Intl.RelativeTimeFormat(defaultLocale, { numeric: 'auto' }).format(-7, 'day'));
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
  it('`currency` formats as currency, whatever a layer names as the style', () => {
    const { resolve } = createParser({ modifierDefaults: { currency: { currency: 'USD', style: 'percent' } } });
    const expected = new Intl.NumberFormat(defaultLocale, { style: 'currency', currency: 'USD' }).format(0.5);

    expect(resolve('{{v:currency}}', { payload: { v: 0.5 }, locale: defaultLocale })).toBe(expected);
    expect(resolve('{{v:currency}}', { payload: { v: 0.5 }, props: { currency: { style: 'percent' } }, locale: defaultLocale })).toBe(expected);
    expect(resolve('{{v:currency}}', { payload: { v: { value: 0.5, props: { currency: { style: 'decimal' } } } }, locale: defaultLocale })).toBe(expected);
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
  it('the modifier registry holds modifiers alone', () => {
    const reports: Report[] = [];
    const { resolve } = createParser({ onReport: (entry) => reports.push(entry) });

    expect(resolve('{{v:agoMap; default:D}}', { payload: { v: 'X' } })).toBe('D');
    expect(reports.map(({ code }) => code)).toEqual(['unknown-modifier']);
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
  it('a value that cannot become text is reported, and one nobody passed is not', () => {
    const reports: Report[] = [];
    const { resolve } = createParser({ onReport: (report) => { reports.push(report); } });

    expect(resolve('{{v; default:INLINE}}', { payload: { v: circular }, key: 'common.opaque' })).toBe('INLINE');

    expect(reports).toHaveLength(1);
    expect(reports[0]).toEqual({
      code: 'unserializable-value',
      message: 'A payload value could not become text, so resolution read it as missing.',
      key: 'common.opaque',
      text: '{{v; default:INLINE}}',
    });

    expect(resolve('{{v; default:INLINE}}', { payload: { v: circular, default: new Opaque() } })).toBe('INLINE');

    expect(reports.map(({ code }) => code)).toEqual(['unserializable-value', 'unserializable-value', 'unserializable-value']);

    expect(resolve('{{v; default:INLINE}}', { payload: {} })).toBe('INLINE');
    expect(resolve('{{v}}', { payload: { v: { value: undefined } } })).toBe('');
    expect(resolve('{{v}}', { payload: { v: 'TEXT' } })).toBe('TEXT');

    expect(reports).toHaveLength(3);
  });
  it('a default nobody reads is never consulted, and one that is read still reports', () => {
    const reports: Report[] = [];
    const { resolve } = createParser({ onReport: (report) => { reports.push(report); } });

    expect(resolve('{{v}} {{v}} {{v}}', { payload: { v: 'A', default: circular } })).toBe('A A A');

    expect(reports).toHaveLength(0);

    expect(resolve('{{v}}', { payload: { default: circular } })).toBe('');

    expect(reports.map(({ code }) => code)).toEqual(['unserializable-value']);
  });
  it('a modifier reads a payload default as text, whatever type the payload gave it', () => {
    const seen: unknown[] = [];
    const { resolve } = createParser({ customModifiers: { test: ({ defaultValue }) => { seen.push(defaultValue); return 'DONE'; } } });

    expect(resolve('{{value:test}}', { payload: { value: 'V', default: 0 } })).toBe('DONE');
    expect(resolve('{{value:test}}', { payload: { value: 'V', default: false } })).toBe('DONE');
    expect(resolve('{{value:test}}', { payload: { value: 'V', default: null } })).toBe('DONE');
    expect(resolve('{{value:test}}', { payload: { value: 'V', default: [1, 2] } })).toBe('DONE');
    expect(resolve('{{value:test; default:INLINE}}', { payload: { value: 'V' } })).toBe('DONE');

    expect(seen).toEqual(['0', 'false', 'null', '[1,2]', 'INLINE']);
  });
  it('a payload default a modifier cannot turn into text still fails soft', () => {
    const { resolve } = defaultParser;

    expect(resolve('{{value; 1:ONE}}', { payload: { value: 2, default: new Opaque() } })).toBe('');
    expect(resolve('{{value}}', { payload: { default: circular } })).toBe('');
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
    const raising = { get a() { throw new Error('TO STRING FAILURE'); } };
    const nothing = { toJSON: () => undefined };

    expect(resolve(circular)).toBe('');
    expect(resolve(new Opaque())).toBe('');
    expect(resolve(raising)).toBe('');
    expect(resolve(nothing)).toBe('');

    expect(resolve('{{value}}', { payload: { value: circular } })).toBe('');
    expect(resolve('{{value}}', { payload: { value: raising, default: 'FALLBACK' } })).toBe('FALLBACK');
    expect(resolve('{{value}}', { payload: { value: nothing, default: 'FALLBACK' } })).toBe('FALLBACK');
    expect(resolve('{{value}}', { payload: { value: 'TEST_STRING', default: circular } })).toBe('TEST_STRING');

    const { resolve: resolveThrowing } = createParser({ customModifiers: { test: () => { throw new Error('MODIFIER FAILURE'); } } });

    expect(resolveThrowing('{{value:test}}', { payload: { value: 'TEST_STRING', default: new Opaque() } })).toBe('');

    const { resolve: resolveOpaque } = createParser({ customModifiers: { test: () => new Opaque() } });

    expect(resolveOpaque('{{value:test; default:FALLBACK}}', { payload: { value: 'TEST_STRING' } })).toBe('FALLBACK');
  });
  it('a message no conversion can describe is a message that does not exist', () => {
    const { resolve } = defaultParser;
    const payload = { default: 'DEFAULT VALUE' };

    expect(resolve(circular, { payload, key: 'common.key' })).toBe('DEFAULT VALUE');
    expect(resolve(new Opaque(), { payload, key: 'common.key' })).toBe('DEFAULT VALUE');
    expect(resolve(circular, { key: 'common.key' })).toBe('common.key');
    expect(resolve(new Opaque(), { key: 'common.key' })).toBe('common.key');

    // A message a conversion does describe is still the message.
    expect(resolve({ a: 1 }, { payload, key: 'common.key' })).toBe('{"a":1}');
    expect(resolve(42, { payload, key: 'common.key' })).toBe('42');
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
  it('a payload entry the host will not describe resolves to the fallback chain', () => {
    const { resolve } = defaultParser;
    const { proxy, revoke } = Proxy.revocable({}, {});

    revoke();

    expect(resolve('{{v; default:FALLBACK}}', { payload: { v: proxy } })).toBe('FALLBACK');
    expect(resolve('{{v; default:FALLBACK}}', { payload: { v: new Proxy({}, { ownKeys: () => { throw new Error('OWN KEYS FAILURE'); } }) } })).toBe('FALLBACK');
  });
  it('a wrapper prop that raises when read reads as missing', () => {
    const { resolve } = defaultParser;
    const value = 1234.56;

    expect(resolve('{{v:number}}', {
      payload: { v: { value, props: { number: { get maximumFractionDigits(): number { throw new Error('WRAPPER PROP FAILURE'); } } } } },
      props: { number: { useGrouping: true } },
      locale: defaultLocale,
    })).toBe(new Intl.NumberFormat(defaultLocale, { useGrouping: true }).format(value));
  });
  it('a formatting modifier that cannot format its input resolves to the fallback chain', () => {
    const resolve = resolverFor<{ value?: any, default?: string }>(defaultLocale);

    expect(resolve('common.modifier_number_default', { value: 'not a number' })).toBe('FALLBACK');
    expect(resolve('common.modifier_currency_default', { value: 'not a number' }, { currency: { currency: 'USD', ratio: 1 } })).toBe('FALLBACK');
    expect(resolve('common.modifier_date', { value: 'not a number', default: 'FALLBACK' })).toBe('FALLBACK');
    expect(resolve('common.modifier_ago', { value: 'not a number', default: 'FALLBACK' })).toBe('FALLBACK');
    expect(resolve('common.modifier_date', { value: 'not a number' })).toBe('');
  });
  it('blank text is not a number a formatting modifier can format', () => {
    const { resolve } = defaultParser;
    const props = { currency: { currency: 'USD', ratio: 21.4 } };

    for (const value of ['', '   ']) {
      expect(resolve('{{v:number; default:FALLBACK}}', { payload: { v: value }, locale: defaultLocale })).toBe('FALLBACK');
      expect(resolve('{{v:date; default:FALLBACK}}', { payload: { v: value }, locale: defaultLocale })).toBe('FALLBACK');
      expect(resolve('{{v:ago; default:FALLBACK}}', { payload: { v: value }, locale: defaultLocale })).toBe('FALLBACK');
      expect(resolve('{{v:currency; default:FALLBACK}}', { payload: { v: value }, props, locale: defaultLocale })).toBe('FALLBACK');
    }
  });
  it('a payload value of only whitespace is not a number a formatting modifier can format', () => {
    const { resolve } = defaultParser;
    const props = { currency: { currency: 'USD', ratio: 21.4 } };

    // No arm for a code point outside the class: it is not blank, so it reaches
    // the host's numeric conversion, reads as `NaN`, and takes the same
    // fallback for the opposite reason.
    for (const space of WHITESPACE) {
      expect(resolve('{{v:number; default:FALLBACK}}', { payload: { v: space }, locale: defaultLocale })).toBe('FALLBACK');
      expect(resolve('{{v:date; default:FALLBACK}}', { payload: { v: space }, locale: defaultLocale })).toBe('FALLBACK');
      expect(resolve('{{v:ago; default:FALLBACK}}', { payload: { v: space }, locale: defaultLocale })).toBe('FALLBACK');
      expect(resolve('{{v:currency; default:FALLBACK}}', { payload: { v: space }, props, locale: defaultLocale })).toBe('FALLBACK');
    }
  });
  it('a numeric comparison converts a blank payload value like any other text', () => {
    const { resolve } = defaultParser;

    for (const space of WHITESPACE) {
      expect(resolve('{{v:lt; 1:X; default:D}}', { payload: { v: space } })).toBe('X');
      expect(resolve('{{v:gt; -1:X; default:D}}', { payload: { v: space } })).toBe('X');
    }

    for (const character of NOT_WHITESPACE) {
      expect(resolve('{{v:lt; 1:X; default:D}}', { payload: { v: character } })).toBe('D');
      expect(resolve('{{v:gt; -1:X; default:D}}', { payload: { v: character } })).toBe('D');
    }
  });
  it('`date` reads a date string, not only a timestamp', () => {
    const { resolve } = defaultParser;
    const stamp = Date.parse('2024-03-05T10:00:00.000Z');
    const formatted = new Intl.DateTimeFormat(defaultLocale, {}).format(stamp);

    expect(resolve('{{v:date; default:FALLBACK}}', { payload: { v: `${stamp}` }, locale: defaultLocale })).toBe(formatted);
    expect(resolve('{{v:date; default:FALLBACK}}', { payload: { v: '2024-03-05T10:00:00.000Z' }, locale: defaultLocale })).toBe(formatted);
    expect(resolve('{{v:date; default:FALLBACK}}', { payload: { v: String(new Date(stamp)) }, locale: defaultLocale })).toBe(formatted);
    expect(resolve('{{v:date; default:FALLBACK}}', { payload: { v: new Date(stamp) }, locale: defaultLocale })).toBe(formatted);

    expect(resolve('{{v:date; default:FALLBACK}}', { payload: { v: 'tomorrow' }, locale: defaultLocale })).toBe('FALLBACK');
    expect(resolve('{{v:date; default:FALLBACK}}', { payload: { v: '' }, locale: defaultLocale })).toBe('FALLBACK');

    expect(resolve('{{v:number; default:FALLBACK}}', { payload: { v: '2024-03-05T10:00:00.000Z' }, locale: defaultLocale })).toBe('FALLBACK');
    expect(resolve('{{v:ago; default:FALLBACK}}', { payload: { v: '2024-03-05T10:00:00.000Z' }, locale: defaultLocale })).toBe('FALLBACK');
    expect(resolve('{{v:currency; default:FALLBACK}}', { payload: { v: '2024-03-05T10:00:00.000Z' }, props: { currency: { currency: 'USD' } }, locale: defaultLocale })).toBe('FALLBACK');
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

    expect(resolve('common.modifier_short_option')).toBe('VALUES: DEF, DEF, DEF');
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
  it('an empty placeholder names no key and resolves to the fallback chain', () => {
    const { resolve } = defaultParser;

    expect(resolve('{{}}')).toBe('');
    expect(resolve('{{}}', { payload: { default: 'D' } })).toBe('D');
    expect(resolve('{{ }}', { payload: { default: 'D' } })).toBe('D');
  });
  it('a backslash before the opening pair writes it as text', () => {
    const { resolve } = defaultParser;

    expect(resolve('\\{{v}}', { payload: { v: 'HIT' } })).toBe('{{v}}');
    expect(resolve('\\{{v}}')).toBe('{{v}}');
    expect(resolve('\\\\{{v}}', { payload: { v: 'HIT' } })).toBe('\\HIT');
  });
  it('a backslash before the closing pair writes it as text', () => {
    const { resolve } = defaultParser;

    expect(resolve('{{v\\}}', { payload: { v: 'HIT' } })).toBe('{{v}}');
    expect(resolve('{{v\\}}}', { payload: { 'v}': 'HIT' } })).toBe('HIT');
    expect(resolve('{{v\\\\}}', { payload: { 'v\\': 'HIT' } })).toBe('HIT');
    expect(resolve('{{a\\}b}}', { payload: { 'a}b': 'HIT' } })).toBe('HIT');
    expect(resolve('{{v\\{}}', { payload: { 'v{': 'HIT' } })).toBe('HIT');
  });
  it('an opening pair inside a placeholder ends the attempt', () => {
    const { resolve } = defaultParser;
    const payload = { 'a{{b': 'NESTKEY', b: 'B', '{v': 'BRACEV' };

    // Only the first brace of the rejected pair becomes text; the scan resumes
    // at the very next code point, where the inner pair opens for itself.
    expect(resolve('{{a{{b}}', { payload })).toBe('{{aB');
    expect(resolve('x {{a{{b}} y', { payload })).toBe('x {{aB y');
    expect(resolve('{{{{v}}', { payload })).toBe('{BRACEV');

    // A backslash claims the brace after it, so what is left is not a pair and
    // the attempt runs on to name a key nobody could name otherwise.
    expect(resolve('{{a\\{{b}}', { payload })).toBe('NESTKEY');
  });
  it('a placeholder holds no line terminator in any position', () => {
    const { resolve } = defaultParser;

    for (const terminator of LINE_TERMINATORS) {
      expect(resolve(`{{${terminator}v${terminator}}}`, { payload: { v: 'HIT' } })).toBe(`{{${terminator}v${terminator}}}`);
      expect(resolve(`{{a}} {{${terminator}v${terminator}}}`, { payload: { a: 'A', v: 'HIT' } })).toBe(`A {{${terminator}v${terminator}}}`);
      expect(resolve(`{{v; a:${terminator}b; default:D}}`, { payload: { v: 'HIT' } })).toBe(`{{v; a:${terminator}b; default:D}}`);
      expect(resolve(`{{a}} {{v; a:${terminator}b; default:D}}`, { payload: { a: 'A', v: 'HIT' } })).toBe(`A {{v; a:${terminator}b; default:D}}`);
      expect(resolve(`{{v\\${terminator}x}}`, { payload: { [`v${terminator}x`]: 'HIT' } })).toBe(`{{v${terminator}x}}`);
    }
  });
  it('the implementation reads `line-term` from the set spelled out above', () => {
    expect([...LINE_TERM].map(codePoint).sort()).toEqual(LINE_TERMINATORS.map(codePoint).sort());
  });
  it('escaped whitespace is text, not padding', () => {
    const { resolve } = defaultParser;

    expect(resolve('{{v; 1:ONE\\ }}', { payload: { v: 1 } })).toBe('ONE ');
    expect(resolve('{{v; 1:\\ ONE}}', { payload: { v: 1 } })).toBe(' ONE');
    expect(resolve('{{v\\ x; 1:ONE}}', { payload: { 'v x': 1 } })).toBe('ONE');
    expect(resolve('{{v; \\ :X; default:D}}', { payload: { v: ' ' } })).toBe('X');
    expect(resolve('{{v; \\ :X; default:D}}', { payload: { v: 'x' } })).toBe('D');
  });
  it('only the whitespace class is trimmed around a key', () => {
    const { resolve } = defaultParser;

    for (const space of PLACEHOLDER_WHITESPACE) {
      expect(resolve(`{{${space}v${space}}}`, { payload: { [`${space}v${space}`]: 'PADDED', v: 'HIT' } })).toBe('HIT');
    }

    for (const character of NOT_WHITESPACE) {
      expect(resolve(`{{${character}v${character}}}`, { payload: { [`${character}v${character}`]: 'PADDED', v: 'HIT' } })).toBe('PADDED');
    }
  });
  it('only the whitespace class is trimmed around a modifier name', () => {
    const { resolve } = defaultParser;

    for (const space of PLACEHOLDER_WHITESPACE) {
      expect(resolve(`{{v:${space}test${space}; default:D}}`, { payload: { v: 'HIT' } })).toBe('HIT');
    }

    for (const character of NOT_WHITESPACE) {
      expect(resolve(`{{v:${character}test${character}; default:D}}`, { payload: { v: 'HIT' } })).toBe('D');
    }
  });
  it('only the whitespace class is trimmed around an option key', () => {
    const { resolve } = defaultParser;

    for (const space of PLACEHOLDER_WHITESPACE) {
      expect(resolve(`{{v;${space}1${space}:ONE; default:D}}`, { payload: { v: 1 } })).toBe('ONE');
    }

    for (const character of NOT_WHITESPACE) {
      expect(resolve(`{{v;${character}1${character}:ONE; default:D}}`, { payload: { v: 1 } })).toBe('D');
    }
  });
  it('only the whitespace class is trimmed around an option value and an inline default', () => {
    const { resolve } = defaultParser;

    for (const space of PLACEHOLDER_WHITESPACE) {
      expect(resolve(`{{v; 1:${space}ONE${space}}}`, { payload: { v: 1 } })).toBe('ONE');
      expect(resolve(`{{v; default:${space}D${space}}}`, { payload: {} })).toBe('D');
    }

    for (const character of NOT_WHITESPACE) {
      expect(resolve(`{{v; 1:${character}ONE${character}}}`, { payload: { v: 1 } })).toBe(`${character}ONE${character}`);
      expect(resolve(`{{v; default:${character}D${character}}}`, { payload: {} })).toBe(`${character}D${character}`);
    }
  });
  it('escaped whitespace is text at every code point the class enumerates', () => {
    const { resolve } = defaultParser;

    for (const space of PLACEHOLDER_WHITESPACE) {
      expect(resolve(`{{v; 1:ONE\\${space}}}`, { payload: { v: 1 } })).toBe(`ONE${space}`);
      expect(resolve(`{{v; 1:\\${space}ONE}}`, { payload: { v: 1 } })).toBe(`${space}ONE`);
      expect(resolve(`{{v\\${space}x; 1:ONE; default:D}}`, { payload: { [`v${space}x`]: 1 } })).toBe('ONE');
      expect(resolve(`{{v; \\${space}:X; default:D}}`, { payload: { v: space } })).toBe('X');
    }

    for (const character of NOT_WHITESPACE) {
      expect(resolve(`{{v; 1:ONE\\${character}}}`, { payload: { v: 1 } })).toBe(`ONE\\${character}`);
      expect(resolve(`{{v; 1:\\${character}ONE}}`, { payload: { v: 1 } })).toBe(`\\${character}ONE`);
      expect(resolve(`{{v\\${character}x; 1:ONE; default:D}}`, { payload: { [`v${character}x`]: 1 } })).toBe('D');
      expect(resolve(`{{v; \\${character}:X; default:D}}`, { payload: { v: character } })).toBe('D');
    }
  });
  it('a backslash escapes the whitespace class and nothing outside it', () => {
    const { resolve } = defaultParser;

    for (const space of WHITESPACE) {
      expect(resolve(`a\\${space}b`)).toBe(`a${space}b`);
    }

    for (const character of NOT_WHITESPACE) {
      expect(resolve(`a\\${character}b`)).toBe(`a\\${character}b`);
    }
  });
  it('a valueless option is trimmed by the whitespace class', () => {
    const { resolve } = defaultParser;

    for (const space of PLACEHOLDER_WHITESPACE) {
      expect(resolve(`{{v:ne;${space}z${space}; default:D}}`, { payload: { v: 'a' } })).toBe('z');
      expect(resolve(`{{v;${space}; default:D}}`, { payload: { v: 1 } })).toBe('1');
    }

    for (const character of NOT_WHITESPACE) {
      expect(resolve(`{{v:ne;${character}z${character}; default:D}}`, { payload: { v: 'a' } })).toBe(`${character}z${character}`);
      expect(resolve(`{{v;${character}; default:D}}`, { payload: { v: 1 } })).toBe('D');
    }
  });
  it('a placeholder part made only of whitespace trims away to nothing', () => {
    const { resolve } = defaultParser;

    for (const space of PLACEHOLDER_WHITESPACE) {
      expect(resolve(`{{v; 1:${space}; default:D}}`, { payload: { v: 1 } })).toBe('');
      expect(resolve(`{{v:${space}}}`, { payload: { v: 'HIT' } })).toBe('HIT');
    }

    for (const character of NOT_WHITESPACE) {
      expect(resolve(`{{v; 1:${character}; default:D}}`, { payload: { v: 1 } })).toBe(character);
      expect(resolve(`{{v:${character}}}`, { payload: { v: 'HIT' } })).toBe('');
    }
  });
  it('a run of whitespace trims like a single member', () => {
    const { resolve } = defaultParser;
    const run = PLACEHOLDER_WHITESPACE.join('');

    expect(resolve(`{{${run}v${run}}}`, { payload: { v: 'HIT' } })).toBe('HIT');
    expect(resolve(`{{v; 1:${run}ONE${run}}}`, { payload: { v: 1 } })).toBe('ONE');
    expect(resolve('{{v:number; default:FALLBACK}}', { payload: { v: WHITESPACE.join('') }, locale: defaultLocale })).toBe('FALLBACK');
  });
  it('the whitespace class is `line-term` and twenty-one more, and nothing else in the plane', () => {
    const { resolve } = defaultParser;
    // Spelled out, not filtered from `WHITESPACE`: that constant would absorb
    // whatever `line-term` gave up, and the expectation would never move.
    const besideLineTerm = [
      '\u0009', '\u000b', '\u000c', '\u0020', '\u00a0', '\u1680', '\u2000', '\u2001',
      '\u2002', '\u2003', '\u2004', '\u2005', '\u2006', '\u2007', '\u2008', '\u2009',
      '\u200a', '\u202f', '\u205f', '\u3000', '\ufeff',
    ];
    const reserved = [':', ';', '{', '}', '\\'];
    const expected = [...LINE_TERMINATORS, ...besideLineTerm].map(codePoint).sort();
    const measured: string[] = [];

    for (let code = 0x0000; code <= 0xffff; code += 1) {
      const character = String.fromCharCode(code);

      if (reserved.includes(character)) continue;

      const text = `{{${character}v${character}}}`;
      const output = resolve(text, { payload: { v: 'HIT' } });

      if (output === text || output === 'HIT') measured.push(codePoint(character));
    }

    expect(measured).toEqual(expected);
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
  it('`default` is reserved in lowercase only', () => {
    const { resolve } = defaultParser;

    expect(resolve('{{v; DEFAULT:UPPER; default:LOWER}}')).toBe('LOWER');
    expect(resolve('{{v; Default:X}}')).toBe('');
    expect(resolve('{{v; DEFAULT:X}}', { payload: { v: 'DEFAULT' } })).toBe('X');
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

    for (const terminator of LINE_TERMINATORS) {
      expect(resolve('common.placeholder_chain', { v1: `{{v1}}${terminator}[i18n]: FORGED${'x'.repeat(1000)}` })).toContain('[i18n]: FORGED');
    }

    expect(reports).toHaveLength(LINE_TERMINATORS.length);

    for (const report of reports) {
      expect(report.text.length).toBeLessThan(300);
      for (const terminator of LINE_TERMINATORS) expect(report.text).not.toContain(terminator);
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
  it('an `onReport` that throws does not take the resolution down', () => {
    const seen: string[] = [];
    const { resolve } = createParser({
      onReport: ({ code }) => {
        seen.push(code);

        throw new Error('the observer blew up');
      },
    });

    expect(resolve('{{v:zz; default:D}}', { payload: { v: '1' }, locale: defaultLocale })).toBe('D');
    expect(seen).toEqual(['unknown-modifier']);
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
  it('merging a large wrapper `props` costs linear time', () => {
    const { resolve } = defaultParser;

    const runAt = (size: number) => {
      const props = Object.fromEntries(Array.from({ length: size }, (_, index) => [`p${index}`, {}]));
      const payload = { v: { value: 'A', props } };

      return () => { resolve('{{v:test}}', { payload, props: {} }); };
    };

    expect(growthWhenInputQuadruples(runAt, 500)).toBeLessThan(3);
  }, 30000);
  it('a default nobody reads is never serialized', () => {
    const { resolve } = defaultParser;

    const runWith = (payloadDefault: unknown) => {
      const payload = { v: 'A', default: payloadDefault };
      const placeholders = '{{v}}'.repeat(200);

      return () => { resolve(placeholders, { payload }); };
    };

    const large = Object.fromEntries(Array.from({ length: 20000 }, (_, index) => [`k${index}`, index]));

    // Serializing that default once per placeholder puts the ratio in the
    // hundreds; the budget is loose enough to absorb a slow CI leg.
    expect(timePerOp(runWith(large)) / timePerOp(runWith('S'))).toBeLessThan(5);
  }, 30000);
});
