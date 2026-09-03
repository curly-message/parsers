import { describe, expect, it } from 'vitest';
import { createParser, Parser, Report } from '../../src';
import { getDateInput, getModifierInput, LINE_TERM } from '../../src/utils';
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

// Four values no conversion can describe: `JSON.stringify` raises on the
// circular one and on the one whose own read raises, `String` raises on the
// class instance, which its prototype keeps off the JSON path, and the last
// serializes to nothing at all.
const circular: Record<string, unknown> = {};

circular.self = circular;

class Opaque {
  toString(): string {
    throw new Error('NO TEXT');
  }
}

const raisingRead = { get a(): never { throw new Error('TO STRING FAILURE'); } };
const noDescription = { toJSON: () => undefined };

// What a caller reads when somebody else has written to the prototype every
// object inherits from. The name is removed again whatever the read does, so
// one test's pollution never reaches the next.
const polluted = (name: string, value: any, read: () => string) => {
  const proto = Object.prototype as any;

  proto[name] = value;

  try {
    return read();
  } finally {
    delete proto[name];
  }
};

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
  it('a message carrying no placeholder resolves to its own text', () => {
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
  it('an inline default fills a placeholder whose key the payload does not carry', () => {
    const resolve = resolverFor<{ value?: any }>(defaultLocale);

    expect(resolve('common.placeholder_default')).toBe('VALUES: DEFAULT_VALUE, DEFAULT_VALUE, DEFAULT_VALUE, DEFAULT_VALUE');
  });
  it('the payload\'s `default` fills a placeholder whose key it does not carry', () => {
    const resolve = resolverFor<{ value?: any }>(defaultLocale);

    expect(resolve('common.placeholder', { default: 'DYNAMIC_DEFAULT_VALUE' })).toBe('VALUES: DYNAMIC_DEFAULT_VALUE, DYNAMIC_DEFAULT_VALUE, DYNAMIC_DEFAULT_VALUE, DYNAMIC_DEFAULT_VALUE');
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
  it('an entry that is not a plain object is a value, wrapper-shaped or not', () => {
    class Wrapped {
      value = 'V';
    }

    const { resolve } = defaultParser;

    expect(resolve('{{v}}', { payload: { v: new Wrapped() } })).toBe('[object Object]');
    expect(resolve('{{v}}', { payload: { v: Object.assign(Object.create({ inherited: 1 }), { value: 'V' }) } })).toBe('[object Object]');
    expect(resolve('{{v}}', { payload: { v: Object.assign(Object.create(null), { value: 'V' }) } })).toBe('V');
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
  it('a resolved default is text the next pass reads, placeholders included', () => {
    const { resolve } = defaultParser;

    expect(resolve('{{v}}', { payload: { v: { default: 'W-{{n}}' }, n: 'N' } })).toBe('W-N');
    expect(resolve('{{v}}', { payload: { default: 'P-{{n}}', n: 'N' } })).toBe('P-N');
    expect(resolve('{{v:eq; z:Z}}', { payload: { v: { value: 'q', default: 'W-{{n}}' }, n: 'N' } })).toBe('W-N');
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
  it('the comparison lower-cases, so it folds nothing and reads no locale', () => {
    const { resolve } = defaultParser;

    // U+00DF written as an escape: a full case fold maps it to `ss` and would
    // select the option, where a lower case mapping leaves it standing.
    expect(resolve('{{v:eq; STRASSE:X; default:D}}', { payload: { v: 'stra\u00dfe' } })).toBe('D');
    expect(resolve('{{v:ne; STRASSE:X; default:D}}', { payload: { v: 'stra\u00dfe' } })).toBe('X');

    // A locale-tailored mapping takes `I` to a dotless `i` under Turkish and
    // Azerbaijani, which would lose the selection every other locale makes.
    ['en', 'cs', 'tr', 'tr-TR', 'az', 'lt'].forEach((locale) => {
      expect(resolve('{{v:eq; I:X; default:D}}', { payload: { v: 'i' }, locale })).toBe('X');
      expect(resolve('{{v:ne; I:X; default:D}}', { payload: { v: 'i' }, locale })).toBe('D');
    });
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
  it('a numeric comparison breaks a tie by source order', () => {
    const { resolve } = defaultParser;

    // Two keys spelled differently that carry the same numeric value tie, and
    // the ordering leaves them as the message wrote them: whichever of the two
    // the message wrote first is the one selected, in either direction.
    expect(resolve('{{v:lt; 2:FIRST; 2.0:SECOND; default:D}}', { payload: { v: 1 } })).toBe('FIRST');
    expect(resolve('{{v:lt; 2.0:FIRST; 2:SECOND; default:D}}', { payload: { v: 1 } })).toBe('FIRST');
    expect(resolve('{{v:gt; 2:FIRST; 2.0:SECOND; default:D}}', { payload: { v: 3 } })).toBe('FIRST');
    expect(resolve('{{v:gt; 2.0:FIRST; 2:SECOND; default:D}}', { payload: { v: 3 } })).toBe('FIRST');
  });
  it('a two-legged comparison runs equality over every option, numeric or not', () => {
    const { resolve } = defaultParser;

    // The equality leg reads the options as the message wrote them, so a key
    // no numeric ordering could hold is still one it can select.
    expect(resolve('{{v:lte; abc:X; default:D}}', { payload: { v: 'abc' } })).toBe('X');
    expect(resolve('{{v:gte; abc:X; default:D}}', { payload: { v: 'abc' } })).toBe('X');
    expect(resolve('{{v:lte; 10:TEN; abc:X; default:D}}', { payload: { v: 'abc' } })).toBe('X');
    expect(resolve('{{v:gte; abc:X; 5:FIVE; default:D}}', { payload: { v: 'abc' } })).toBe('X');
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
  it('every formatting modifier layers its `props` over its own defaults', () => {
    const stamp = Date.parse('2024-03-05T10:00:00.000Z');
    const days = -2 * 24 * 60 * 60 * 1000;

    const dates = createParser({ modifierDefaults: { date: { dateStyle: 'full' } } });
    const agos = createParser({ modifierDefaults: { ago: { style: 'long' } } });
    const currencies = createParser({ modifierDefaults: { currency: { currency: 'USD', currencyDisplay: 'name' } } });

    expect(dates.resolve('{{v:date}}', { payload: { v: stamp }, props: { date: { dateStyle: 'short' } }, locale: defaultLocale }))
      .toBe(new Intl.DateTimeFormat(defaultLocale, { dateStyle: 'short' }).format(stamp));
    expect(agos.resolve('{{v:ago}}', { payload: { v: days }, props: { ago: { style: 'narrow' } }, locale: defaultLocale }))
      .toBe(new Intl.RelativeTimeFormat(defaultLocale, { numeric: 'auto', style: 'narrow' }).format(-2, 'day'));
    expect(currencies.resolve('{{v:currency}}', { payload: { v: 10 }, props: { currency: { currencyDisplay: 'code' } }, locale: defaultLocale }))
      .toBe(new Intl.NumberFormat(defaultLocale, { style: 'currency', currency: 'USD', currencyDisplay: 'code' }).format(10));
  });
  it('a wrapper leaves every prop it does not name alone', () => {
    const seen: unknown[] = [];
    const { resolve } = createParser<{ v: any }, { test?: Intl.NumberFormatOptions }>({ customModifiers: { test: ({ props }) => { seen.push(props); return 'DONE'; } } });

    expect(resolve('{{v:test}}', { payload: { v: { value: 1, props: { test: { maximumFractionDigits: 1 } } } }, props: { test: { useGrouping: true }, date: { timeStyle: 'full' } } })).toBe('DONE');
    expect(resolve('{{v:test}}', { payload: { v: 1 }, props: { test: { useGrouping: true } } })).toBe('DONE');

    expect(seen).toEqual([
      { useGrouping: true, maximumFractionDigits: 1 },
      { useGrouping: true },
    ]);
  });
  it('a modifier reads the slice its own name holds, layered over every source', () => {
    const seen: unknown[] = [];
    const table = { test: ({ props }: { props?: unknown }) => { seen.push(props); return 'DONE'; } };
    const { resolve } = createParser<{ v: any }, { test?: Intl.NumberFormatOptions }>({
      customModifiers: table,
      modifierDefaults: { test: { maximumFractionDigits: 4, useGrouping: false }, number: { style: 'percent' } },
    });

    // A host-defined modifier is configured through the layers a built-in one
    // is, `modifierDefaults` included, and reads what its own name holds in
    // them — never what another modifier was configured with, and never the
    // table those layers are.
    expect(resolve('{{v:test}}', { payload: { v: { value: 1, props: { test: { maximumFractionDigits: 1 } } } }, props: { test: { useGrouping: true }, number: { style: 'decimal' } } })).toBe('DONE');
    expect(resolve('{{v:test}}', { payload: { v: 1 } })).toBe('DONE');
    // A modifier nobody configured reads an object all the same.
    expect(createParser({ customModifiers: table }).resolve('{{v:test}}', { payload: { v: 1 } })).toBe('DONE');

    expect(seen).toEqual([
      { maximumFractionDigits: 1, useGrouping: true },
      { maximumFractionDigits: 4, useGrouping: false },
      {},
    ]);
  });
  it('a wrapper prop set to `undefined` leaves the layer beneath it standing', () => {
    const { resolve } = createParser();
    const value = 1234.56789;

    expect(resolve('{{v:number}}', { payload: { v: { value, props: { number: undefined } } }, props: { number: { useGrouping: false } }, locale: defaultLocale })).toBe('1234.57');
    expect(resolve('{{v:number}}', { payload: { v: { value, props: { number: { useGrouping: undefined } } } }, props: { number: { useGrouping: false, maximumFractionDigits: 3 } }, locale: defaultLocale })).toBe('1234.568');
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
  it('a `modifierDefaults` prop set to `undefined` leaves the modifier\'s own default standing', () => {
    const { resolve } = createParser({ modifierDefaults: { number: { maximumFractionDigits: undefined } } });

    // `modifierDefaults` is the bottom layer a caller writes, and the layer
    // under it is the modifier itself: a name it sets to `undefined` names
    // nothing, so the two fraction digits `number` formats stand rather than
    // giving way to what `Intl` would have chosen.
    expect(resolve('{{v:number}}', { payload: { v: 1.23456 }, locale: defaultLocale })).toBe('1.23');
    expect(resolve('{{v:number}}', { payload: { v: 1.23456 }, props: { number: { maximumFractionDigits: 4 } }, locale: defaultLocale })).toBe('1.2346');
  });
  it('a modifier receives a payload-supplied `props` as a copy', () => {
    const seen: any[] = [];
    const { resolve } = createParser<{ v: any }, { test?: Intl.NumberFormatOptions }>({ customModifiers: { test: ({ props }) => { seen.push(props); return 'DONE'; } } });
    const wrapperProps = { test: { maximumFractionDigits: 1 } };

    expect(resolve('{{v:test}}', { payload: { v: { value: 1, props: wrapperProps } } })).toBe('DONE');
    expect(resolve('{{v:test}}', { payload: { v: { value: 1, props: wrapperProps } }, props: { date: { timeStyle: 'full' } } })).toBe('DONE');

    seen.forEach((props) => {
      expect(props).not.toBe(wrapperProps.test);

      props.maximumFractionDigits = 4;
    });

    expect(wrapperProps).toEqual({ test: { maximumFractionDigits: 1 } });
  });
  it('a modifier receives the call\'s `props` as a copy', () => {
    const seen: any[] = [];
    const { resolve } = createParser<{ v: any }, { test?: Intl.NumberFormatOptions }>({ customModifiers: { test: ({ props }) => { seen.push(props); return 'DONE'; } } });
    const callProps = { test: { maximumFractionDigits: 1 } };

    expect(resolve('{{v:test}}', { payload: { v: 1 }, props: callProps })).toBe('DONE');
    expect(resolve('{{v:test}}', { payload: { v: { value: 1 } }, props: callProps })).toBe('DONE');

    seen.forEach((props) => {
      expect(props).not.toBe(callProps.test);

      props.maximumFractionDigits = 4;
    });

    expect(callProps).toEqual({ test: { maximumFractionDigits: 1 } });
  });
  it('a modifier receives its `props` carrying no prototype', () => {
    const seen: any[] = [];
    const { resolve } = createParser<{ v: any }, { test?: Intl.NumberFormatOptions }>({ customModifiers: { test: ({ props }) => { seen.push(props); return 'DONE'; } } });

    // The slice is accumulated onto a null prototype and leaves on one, so a
    // name a message or a payload supplies is an own entry of it rather than a
    // write through a prototype setter, and a modifier reading its properties
    // reads what it was configured with and never what somebody else wrote
    // where every object would have found it.
    expect(resolve('{{v:test}}', { payload: { v: { value: 1, props: { test: { maximumFractionDigits: 1 } } } }, props: { date: { timeStyle: 'full' } } })).toBe('DONE');
    expect(resolve('{{v:test}}', { payload: { v: 1 }, props: { test: { useGrouping: true } } })).toBe('DONE');

    seen.forEach((props) => { expect(Object.getPrototypeOf(props)).toBe(null); });
  });
  it('merging a wrapper\'s `props` cannot reach a prototype', () => {
    const { resolve } = createParser<{ v: any }, { test?: Intl.NumberFormatOptions }>({ customModifiers: { test: ({ props }) => JSON.stringify(props) } });
    const polluting = JSON.parse('{"__proto__":{"polluted":true}}');

    expect(resolve('{{v:test}}', { payload: { v: { value: 1, props: { test: polluting } } }, props: { test: { useGrouping: true } } })).toBe('{"useGrouping":true,"__proto__":{"polluted":true}}');
    expect(({} as any).polluted).toBe(undefined);
  });
  it('a modifier\'s own layer cannot reach a prototype either', () => {
    const { resolve } = createParser({});
    // `JSON.parse` is how a `__proto__` arrives as an own key: an object
    // literal spelling one sets the prototype instead of recording it.
    const injected = (json: string) => JSON.parse(json);

    expect(resolve('{{v:ago}}', {
      payload: { v: -2 * 24 * 60 * 60 * 1000 },
      props: { ago: injected('{"__proto__":{"format":"year","numeric":"always"}}') },
      locale: defaultLocale,
    })).toBe('2 days ago');
    expect(resolve('{{v:currency}}', {
      payload: { v: 10 },
      props: { currency: injected('{"__proto__":{"ratio":100},"currency":"USD"}') },
      locale: defaultLocale,
    })).toBe('$10.00');

    expect(({} as any).ratio).toBe(undefined);
  });
  it('a `props` layer overrides only the names it carries, whatever its prototype', () => {
    const { resolve } = createParser<{ v: any }, { test?: Intl.NumberFormatOptions }>({ customModifiers: { test: ({ props }) => JSON.stringify(props) } });
    const layer: any = Object.create({ inherited: 'INHERITED' });

    layer.maximumFractionDigits = 1;

    const wrapper = { value: 1, props: { test: layer } };
    const inheritedBag: any = Object.create({ date: { month: 'long' } });

    inheritedBag.test = { maximumFractionDigits: 1 };

    expect(resolve('{{v:test}}', { payload: { v: wrapper }, props: { test: { useGrouping: false } } })).toBe('{"useGrouping":false,"maximumFractionDigits":1}');
    expect(resolve('{{v:test}}', { payload: { v: { value: 1, props: inheritedBag } }, props: { test: { useGrouping: false } } })).toBe('{"useGrouping":false,"maximumFractionDigits":1}');
  });
  it('configuration is read as own properties, never through a prototype', () => {
    const payload = { v: 1.23456789 };
    const seen: Report[] = [];

    expect(polluted('customModifiers', { number: () => 'HIJACKED' }, () => createParser({}).resolve('{{v:number}}', { payload, locale: defaultLocale }))).toBe('1.23');
    expect(polluted('modifierDefaults', { number: { maximumFractionDigits: 5 } }, () => createParser({}).resolve('{{v:number}}', { payload, locale: defaultLocale }))).toBe('1.23');
    expect(polluted('number', { maximumFractionDigits: 5 }, () => createParser({ modifierDefaults: {} }).resolve('{{v:number}}', { payload, props: {}, locale: defaultLocale }))).toBe('1.23');
    expect(polluted('onReport', (entry: Report) => seen.push(entry), () => createParser({}).resolve('{{v:nosuch}}', { payload, locale: defaultLocale }))).toBe('');
    // The chain a message resolves through reads the channel for itself, one
    // level above the placeholder, and reads it the same way.
    expect(polluted('onReport', (entry: Report) => seen.push(entry), () => createParser({}).resolve(undefined, { payload: { get default(): never { throw new Error('READ FAILURE'); } }, key: 'common.key' }))).toBe('common.key');
    // The interpolation guards read the channel for themselves too, between the
    // two, and a payload value that references its own placeholder is what
    // reaches one of them.
    expect(polluted('onReport', (entry: Report) => seen.push(entry), () => createParser({}).resolve('{{v}}', { payload: { v: '{{v}}' } }))).toBe('{{v}}');
    expect(seen).toEqual([]);
  });
  it('a polluted prototype configures no formatter', () => {
    const { resolve } = createParser({});

    const stamp = Date.parse('2024-03-05T10:00:00.000Z');
    const days = -2 * 24 * 60 * 60 * 1000;

    const asNumber = () => resolve('{{v:number}}', { payload: { v: 1.23456789 }, locale: defaultLocale });
    const asDate = () => resolve('{{v:date}}', { payload: { v: stamp }, locale: defaultLocale });
    const asAgo = () => resolve('{{v:ago}}', { payload: { v: days }, locale: defaultLocale });
    const asCurrency = () => resolve('{{v:currency}}', { payload: { v: 10 }, props: { currency: { currency: 'USD' } }, locale: defaultLocale });

    // What each modifier renders with nothing on the prototype, read off `Intl`
    // rather than spelled out: a date renders in the host's own zone.
    const clean = [
      new Intl.NumberFormat(defaultLocale, { maximumFractionDigits: 2 }).format(1.23456789),
      new Intl.DateTimeFormat(defaultLocale).format(stamp),
      new Intl.RelativeTimeFormat(defaultLocale, { numeric: 'auto' }).format(-2, 'day'),
      new Intl.NumberFormat(defaultLocale, { style: 'currency', currency: 'USD' }).format(10),
    ];

    expect([asNumber(), asDate(), asAgo(), asCurrency()]).toEqual(clean);

    expect(polluted('style', 'percent', asNumber)).toBe(clean[0]);
    expect(polluted('minimumFractionDigits', 4, asNumber)).toBe(clean[0]);
    expect(polluted('dateStyle', 'full', asDate)).toBe(clean[1]);
    expect(polluted('style', 'narrow', asAgo)).toBe(clean[2]);
    expect(polluted('currencyDisplay', 'name', asCurrency)).toBe(clean[3]);
  });
  it('a polluted prototype writes no limit into a report', () => {
    const reports: Report[] = [];
    const raising = { 'x-raise': () => { throw new Error('MODIFIER FAILURE'); } };
    const { resolve } = createParser({ customModifiers: raising, onReport: (report) => { reports.push(report); } });

    const read = () => {
      reports.length = 0;

      resolve('{{v:nosuch}}', { payload: { v: 'V' }, key: 'common.key' });
      resolve('{{v:x-raise}}', { payload: { v: 'V' }, key: 'common.key' });
      resolve(circular, { payload: { default: circular }, key: 'common.key' });
      resolve('{{v}}', { payload: { v: '{{v}}' }, key: 'common.key' });
      resolve('{{v}}', { payload: { v: 'x'.repeat(100001) }, key: 'common.key' });

      return reports.map(({ code, limit }) => `${code}=${limit}`).join(' ');
    };

    // A report about no limit carries none, and a code that is about no limit
    // is one the table behind that field names with nothing. Read through a
    // prototype, a name somebody else wrote there answers for the table, and a
    // host prints it as the limit this parser reached.
    const clean = 'unknown-modifier=undefined failed-modifier=undefined unserializable-value=undefined pass-limit=10 output-limit=100000';

    expect(read()).toBe(clean);

    for (const code of ['unknown-modifier', 'failed-modifier', 'unserializable-value', 'pass-limit', 'output-limit']) {
      expect(polluted(code, 'HIJACKED', read)).toBe(clean);
    }
  });
  it('every code declares one origin, and a report carries the one its code declares', () => {
    const reports: Report[] = [];
    const raising = { 'x-raise': () => { throw new Error('MODIFIER FAILURE'); } };
    const { resolve } = createParser({ customModifiers: raising, onReport: (report) => { reports.push(report); } });

    // The whole vocabulary, against the table the parser reads: a code added
    // there without an origin stops compiling here rather than reaching a
    // caller carrying none.
    const origins: Record<Report['code'], Report['origin']> = {
      'unknown-modifier': 'message',
      'failed-modifier': 'message',
      'missing-options': 'message',
      'unserializable-value': 'payload',
      'missing-locale': 'payload',
      'pass-limit': 'limit',
      'output-limit': 'limit',
    };

    resolve('{{v:nosuch}}', { payload: { v: 'V' } });
    resolve('{{v:x-raise}}', { payload: { v: 'V' } });
    resolve('{{v:number}}', { payload: { v: 1 } });
    resolve(circular, { payload: { default: circular } });
    resolve('{{v}}', { payload: { v: '{{v}}' } });
    resolve('{{v}}', { payload: { v: 'x'.repeat(100001) } });

    expect(reports.map(({ code }) => code)).toEqual(['unknown-modifier', 'failed-modifier', 'missing-locale', 'unserializable-value', 'pass-limit', 'output-limit']);

    // A report carries the origin its own code declares, so the axis a caller
    // reads is the code's and never the reporting site's.
    for (const { code, origin } of reports) expect(origin).toBe(origins[code]);

    // The one nothing emits yet is declared all the same: the axis belongs to
    // the vocabulary, not to what a resolution has happened to reach.
    expect(origins['missing-options']).toBe('message');
  });
  it('the scanner reads no character past the end of the message', () => {
    const { resolve } = createParser({});
    const messages = ['{{v}', '{{v{', '{{v', 'a{', '{{{}', '{{v:eq}'];
    const resolveAll = () => messages.map((message) => resolve(message, { payload: { v: 'V', default: 'CHAIN' }, key: 'common.key' })).join('|');

    // Both delimiters are two characters, so every scan asks what follows the
    // character it is on, and past the last one that question leaves the
    // message: a `}` somebody else wrote on the prototype closes a placeholder
    // the message left open, and the text a translator wrote resolves away.
    const clean = messages.join('|');

    expect(resolveAll()).toBe(clean);

    for (let index = 0; index <= Math.max(...messages.map(({ length }) => length)); index += 1) {
      for (const character of ['{', '}', '\\']) {
        expect(polluted(`${index}`, character, resolveAll)).toBe(clean);
      }
    }
  });
  it('the segment scan reads no character past the end of a placeholder', () => {
    const reports: Report[] = [];
    const { resolve } = createParser({ onReport: (report) => { reports.push(report); } });
    const messages = ['{{v:eq}}', '{{v:eq; 1:one}}', '{{:}}', '{{v;}}', '{{v; :D}}', '{{v:number}}', '{{v:nosuch}}'];

    const resolveAll = () => {
      reports.length = 0;

      const text = messages.map((each) => resolve(each, { payload: { v: 'V', default: 'CHAIN' }, locale: defaultLocale, key: 'common.key' })).join('|');

      return `${text} ${reports.map(({ code }) => code).join(' ')}`;
    };

    // What a placeholder holds is cut into segments and options by scans over
    // the text between its delimiters, and past the last character those scans
    // ask the prototype: a separator somebody else wrote there cuts a segment
    // the message does not have, and the parser reports a modifier nobody named.
    // `number` over `'V'` reports too — it is a value no locale formats — and
    // what a polluted prototype must not do is move either list.
    const clean = 'CHAIN|CHAIN|CHAIN|V|V|CHAIN|CHAIN failed-modifier unknown-modifier';

    expect(resolveAll()).toBe(clean);

    for (let index = 0; index <= Math.max(...messages.map(({ length }) => length)); index += 1) {
      for (const character of [':', ';']) {
        expect(polluted(`${index}`, character, resolveAll)).toBe(clean);
      }
    }
  });
  it('the call context is read as own properties, never through a prototype', () => {
    const { resolve } = createParser({});

    expect(polluted('payload', { v: 'HIJACKED' }, () => resolve('{{v; default:D}}', {}))).toBe('D');
    expect(polluted('payload', { v: 'HIJACKED' }, () => resolve('{{v; default:D}}'))).toBe('D');
    expect(polluted('payload', { default: 'HIJACKED' }, () => resolve(undefined, { key: 'a.b' }))).toBe('a.b');
    expect(polluted('key', 'HIJACKED', () => resolve(undefined, {}))).toBe('');
    expect(polluted('locale', altLocale, () => resolve('{{v:number}}', { payload: { v: 1.5 } }))).toBe('');
    expect(polluted('props', { number: { minimumFractionDigits: 4 } }, () => resolve('{{v:number}}', { payload: { v: 1.5 }, locale: defaultLocale }))).toBe('1.5');
  });
  it('a context nobody passed and one passed as `null` resolve alike', () => {
    const { resolve } = createParser({});

    expect(resolve('{{v; default:D}}', null as any)).toBe('D');
    expect(resolve('{{v; default:D}}', undefined)).toBe('D');
    expect(resolve(undefined, null as any)).toBe('');
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
    // Under half an hour, so the automatic climb stops at the minute rung.
    const value = -1000 * 60 * 20;

    expect(resolve('common.modifier_ago', { value })).toBe(new Intl.RelativeTimeFormat(defaultLocale, { numeric: 'auto' }).format(-20, 'minute'));
    expect(resolveAlt('common.modifier_ago', { value })).toBe(new Intl.RelativeTimeFormat(altLocale, { numeric: 'auto' }).format(-20, 'minute'));
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
  it('`ago` reads a delta and its negation the same way', () => {
    const { resolve } = createParser({});
    const relative = new Intl.RelativeTimeFormat(defaultLocale, { numeric: 'auto' });
    const at = (value: number) => resolve('{{v:ago}}', { payload: { v: value }, locale: defaultLocale });
    const hour = 1000 * 60 * 60;

    // A half rounds away from zero on both sides, so the two directions choose
    // the same unit and the same count.
    expect(at(1.5 * hour)).toBe(relative.format(2, 'hour'));
    expect(at(-1.5 * hour)).toBe(relative.format(-2, 'hour'));
    expect(at(hour / 2)).toBe(relative.format(1, 'hour'));
    expect(at(-hour / 2)).toBe(relative.format(-1, 'hour'));
    expect(at(500)).toBe(relative.format(1, 'second'));
    expect(at(-500)).toBe(relative.format(-1, 'second'));
  });
  it('`ago` climbs every step of its unit ladder', () => {
    const { resolve } = createParser({});
    const relative = new Intl.RelativeTimeFormat(defaultLocale, { numeric: 'auto' });
    const at = (value: number) => resolve('{{v:ago}}', { payload: { v: value }, locale: defaultLocale });

    // The ladder is spelled out again rather than read from the source: a step
    // derived from the table it is meant to pin would move along with it.
    const second = 1000;
    const minute = 60 * second;
    const hour = 60 * minute;
    const day = 24 * hour;
    const week = 7 * day;
    const month = 13 / 3 * week;
    const year = 12 * month;

    expect(at(2 * second)).toBe(relative.format(2, 'second'));
    expect(at(2 * minute)).toBe(relative.format(2, 'minute'));
    expect(at(2 * hour)).toBe(relative.format(2, 'hour'));
    expect(at(2 * day)).toBe(relative.format(2, 'day'));
    expect(at(2 * week)).toBe(relative.format(2, 'week'));
    expect(at(2 * month)).toBe(relative.format(2, 'month'));
    expect(at(2 * year)).toBe(relative.format(2, 'year'));

    // A doubled step rounds to the same answer for any factor near twelve, so
    // the top rung is pinned where that rounding parts instead.
    expect(at(17 * month)).toBe(relative.format(1, 'year'));
    expect(at(18 * month)).toBe(relative.format(2, 'year'));
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
  it('a formatting modifier reads its properties from `props`, never from an option', () => {
    const { resolve } = defaultParser;
    const usd = (amount: number) => new Intl.NumberFormat(defaultLocale, { style: 'currency', currency: 'USD' }).format(amount);
    const digits = (amount: number, maximumFractionDigits: number) => new Intl.NumberFormat(defaultLocale, { maximumFractionDigits }).format(amount);
    const relative = (delta: number, unit: Intl.RelativeTimeFormatUnit) => new Intl.RelativeTimeFormat(defaultLocale, { numeric: 'auto' }).format(delta, unit);
    const week = -7 * 24 * 60 * 60 * 1000;

    // A placeholder segment spelled like a property is an option, and an
    // option reaches no layer: the ratio stays 1, the maximum stays two, and
    // the unit stays the one the climb selects.
    expect(resolve('{{v:currency; ratio:100}}', { payload: { v: 2 }, props: { currency: { currency: 'USD' } }, locale: defaultLocale })).toBe(usd(2));
    expect(resolve('{{v:currency}}', { payload: { v: 2 }, props: { currency: { currency: 'USD', ratio: 100 } }, locale: defaultLocale })).toBe(usd(200));

    expect(resolve('{{v:number; maximumFractionDigits:4}}', { payload: { v: 1.23456 }, locale: defaultLocale })).toBe(digits(1.23456, 2));
    expect(resolve('{{v:number}}', { payload: { v: 1.23456 }, props: { number: { maximumFractionDigits: 4 } }, locale: defaultLocale })).toBe(digits(1.23456, 4));

    expect(resolve('{{v:ago; format:day}}', { payload: { v: week }, locale: defaultLocale })).toBe(relative(-1, 'week'));
    expect(resolve('{{v:ago}}', { payload: { v: week }, props: { ago: { format: 'day' } }, locale: defaultLocale })).toBe(relative(-7, 'day'));
  });
  it('custom modifier works', () => {
    const resolve = resolverFor<{ data?: any }>(defaultLocale, createParser({
      customModifiers: {
        test: ({ value }) => value,
      },
    }));

    expect(resolve('common.modifier_custom', { data: 'TEST_STRING' })).toBe('TEST_STRING');
  });
  it('a modifier is handed the value as text, its options and the locale', () => {
    const seen: unknown[] = [];
    const { resolve } = createParser({ customModifiers: { 'x-see': ({ value, options, locale }) => { seen.push({ value, options, locale }); return 'DONE'; } } });

    // A modifier receives the value, the options, the default, the locale and
    // the props. The default and the props are handed over above; these are the
    // other three, and no modifier sees a value at the type it was authored
    // with.
    expect(resolve('{{v:x-see; 10:TEN; 2:TWO; abc:X; z; w:; default:D}}', { payload: { v: { a: 1 } }, locale: altLocale })).toBe('DONE');
    expect(resolve('{{v:x-see; a\\:b : X ; \\ :SPACE}}', { payload: { v: 1 }, locale: defaultLocale })).toBe('DONE');
    expect(resolve('{{v:x-see}}', { payload: { v: 'V' } })).toBe('DONE');

    expect(seen).toEqual([
      // Every segment but the inline default, in the order the message wrote
      // them, each key and value unescaped and trimmed the way a key is.
      {
        value: '{"a":1}',
        options: [
          { key: '10', value: 'TEN' },
          { key: '2', value: 'TWO' },
          { key: 'abc', value: 'X' },
          { key: 'z', value: 'z' },
          { key: 'w', value: '' },
        ],
        locale: altLocale,
      },
      { value: '1', options: [{ key: 'a:b', value: 'X' }, { key: ' ', value: 'SPACE' }], locale: defaultLocale },
      // A message that names no option hands over an empty list rather than
      // nothing, and a caller that named no locale hands over none.
      { value: 'V', options: [], locale: undefined },
    ]);
  });
  it('a modifier name selects on its unescaped spelling, like a key', () => {
    const { resolve } = createParser({ customModifiers: { 'x-a:b': () => 'COLON', 'x-c ': () => 'SPACE' } });

    expect(resolve('{{v:x-a\\:b}}', { payload: { v: 1 }, locale: defaultLocale })).toBe('COLON');
    expect(resolve('{{v:x-c\\ }}', { payload: { v: 1 }, locale: defaultLocale })).toBe('SPACE');
  });
  it('a modifier name keeps every colon after the first, like an option value', () => {
    const reports: Report[] = [];
    const { resolve } = createParser({ customModifiers: { 'x-a:b': () => 'COLON' }, onReport: (report) => { reports.push(report); } });

    // The first colon of a declaration separates the key from the modifier and
    // every later one is part of the name, so a name carrying one needs no
    // escape and escaping it names the same modifier. The rule is what keeps a
    // name with something appended to it from running the modifier it starts
    // with.
    expect(resolve('{{v:x-a:b}}', { payload: { v: 1 }, locale: defaultLocale })).toBe('COLON');
    expect(resolve('{{v:x-a\\:b}}', { payload: { v: 1 }, locale: defaultLocale })).toBe('COLON');
    expect(resolve('{{v:number:x; default:D}}', { payload: { v: 1.5 }, locale: defaultLocale })).toBe('D');

    expect(reports.map(({ code }) => code)).toEqual(['unknown-modifier']);
    expect(reports[0].text).toBe('{{v:number:x; default:D}}');
  });
  it('a modifier answers with a host value, converted like any other', () => {
    const { resolve } = createParser({
      customModifiers: {
        'x-void': () => undefined,
        'x-obj': () => ({ a: 1 }),
        'x-arr': () => [1, 2],
        'x-circ': () => circular,
      },
    });
    const payload = { v: 1 };

    // A plain object and an array serialize, the way a payload value does.
    expect(resolve('{{v:x-obj}}', { payload, locale: defaultLocale })).toBe('{"a":1}');
    expect(resolve('{{v:x-arr}}', { payload, locale: defaultLocale })).toBe('[1,2]');

    // An answer that is nothing, or that no conversion can describe, is no
    // answer: the placeholder takes the fallback chain rather than rendering
    // the host's own word for it.
    expect(resolve('{{v:x-void; default:D}}', { payload, locale: defaultLocale })).toBe('D');
    expect(resolve('{{v:x-void}}', { payload, locale: defaultLocale })).toBe('');
    expect(resolve('{{v:x-circ; default:D}}', { payload, locale: defaultLocale })).toBe('D');
  });
  it('an answer no conversion can describe is reported, and one that is nothing is not', () => {
    const reports: Report[] = [];
    const { resolve } = createParser({
      customModifiers: {
        'x-void': () => undefined,
        'x-circ': () => circular,
        'x-opaque': () => new Opaque(),
      },
      onReport: (report) => { reports.push(report); },
    });
    const payload = { v: 1 };

    // A modifier's answer converts the way a payload entry does, so an answer
    // it cannot describe is described as missing the same way one of those is.
    expect(resolve('{{v:x-circ; default:D}}', { payload, locale: defaultLocale, key: 'common.opaque' })).toBe('D');

    expect(reports).toEqual([{
      code: 'unserializable-value',
      origin: 'payload',
      message: 'A value could not become text, so resolution read it as missing.',
      key: 'common.opaque',
      text: '{{v:x-circ; default:D}}',
    }]);

    expect(resolve('{{v:x-opaque; default:D}}', { payload, locale: defaultLocale })).toBe('D');

    expect(reports.map(({ code }) => code)).toEqual(['unserializable-value', 'unserializable-value']);

    // An answer that is nothing at all is an absent answer, not one that could
    // not be described, so it takes the chain without reporting.
    expect(resolve('{{v:x-void; default:D}}', { payload, locale: defaultLocale })).toBe('D');
    expect(resolve('{{v:x-void}}', { payload, locale: defaultLocale })).toBe('');

    expect(reports).toHaveLength(2);
  });
  it('the fallback chain waits for a reader, and reports only what one read', () => {
    const reports: Report[] = [];
    const { resolve } = createParser({ onReport: (report) => { reports.push(report); } });
    const payload = { v: 1, default: circular };

    // The value resolved, so nothing read the chain and nothing went missing.
    expect(resolve('{{v:number}}', { payload, locale: defaultLocale })).toBe('1');
    expect(reports).toHaveLength(0);

    // A two-leg comparison reads the chain no earlier: the strict leg answers
    // where the equality leg did not, and the default stays behind both.
    expect(resolve('{{v:lte; 5:FIVE}}', { payload, locale: defaultLocale })).toBe('FIVE');
    expect(reports).toHaveLength(0);

    // And the equality leg answering does not pay for the strict one either:
    // the strict leg is handed over unread, so a hit leaves the entry behind
    // it unconverted rather than converting it for an answer nobody wanted.
    expect(resolve('{{v:lte; 1:ONE}}', { payload, locale: defaultLocale })).toBe('ONE');
    expect(resolve('{{v:gte; 1:ONE}}', { payload, locale: defaultLocale })).toBe('ONE');
    expect(reports).toHaveLength(0);

    // A selection matching nothing does read it, and there the entry no
    // conversion can describe is a value resolution read as missing.
    expect(resolve('{{v:eq; 2:TWO}}', { payload, locale: defaultLocale })).toBe('');
    expect(reports.map(({ code }) => code)).toEqual(['unserializable-value']);
  });
  it('a placeholder that names `default` reads that entry once', () => {
    const reports: Report[] = [];
    const { resolve } = createParser({ onReport: (report) => { reports.push(report); } });

    // The payload's root `default` is this placeholder's own value, and the
    // chain link behind it is the same entry. One entry that cannot become
    // text is one value that went missing, not two.
    expect(resolve('{{default}}', { payload: { default: circular }, locale: defaultLocale })).toBe('');
    expect(reports.map(({ code }) => code)).toEqual(['unserializable-value']);

    // A modifier that cannot format it reads the chain, and lands back on the
    // text the same entry already gave: the payload still speaks for the key
    // it carries, whatever that key is called.
    expect(resolve('{{default:number}}', { payload: { default: 'abc' }, locale: defaultLocale })).toBe('abc');
  });
  it('a placeholder that asks its fallback chain twice converts it once', () => {
    const reports: Report[] = [];
    const seen: unknown[] = [];
    const { resolve } = createParser({
      customModifiers: {
        'x-void': ({ defaultValue }) => { seen.push(defaultValue); },
        'x-raise': ({ defaultValue }) => { seen.push(defaultValue); throw new Error('MODIFIER FAILURE'); },
      },
      onReport: (report) => { reports.push(report); },
    });
    const payload = { v: 1, default: circular };

    // Only a custom modifier asks twice: every built-in that reads the chain
    // returns what it read, so its answer converts and the placeholder never
    // comes back. One entry that cannot become text is one value that went
    // missing, however many times the placeholder asked for it.
    expect(resolve('{{v:x-void}}', { payload, locale: defaultLocale })).toBe('');
    expect(reports.map(({ code }) => code)).toEqual(['unserializable-value']);

    // And a modifier that raises after reading lands on the same chain, not on
    // a second walk of it. The raise earns a report of its own; what the chain
    // owes is the one entry it could not describe, read once more and not twice.
    expect(resolve('{{v:x-raise; default:INLINE}}', { payload, locale: defaultLocale })).toBe('INLINE');
    expect(reports.map(({ code }) => code)).toEqual(['unserializable-value', 'unserializable-value', 'failed-modifier']);

    expect(seen).toEqual(['', 'INLINE']);
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
      origin: 'message',
      message: 'A placeholder named a modifier this parser does not know.',
      key: 'common.plural',
      text: '{{value:plural; 1:one; default:many}}',
    });
  });
  it('the modifier registry holds modifiers alone', () => {
    const reports: Report[] = [];
    // The types admit modifiers alone, but a JavaScript caller reaches this
    // table with anything, and the parser's own exports once reached it with a
    // data table of their own.
    const registry = { agoMap: [{ key: 'second', multiplier: 1000 }], eq: 'not a modifier' } as any;
    const { resolve } = createParser({ customModifiers: registry, onReport: (entry) => reports.push(entry) });

    expect(resolve('{{v:agoMap; default:D}}', { payload: { v: 'X' } })).toBe('D');
    // An entry that is not a modifier registers none, so it takes no name a
    // message can write and shadows no modifier that already answers to one.
    expect(resolve('{{v; X:HIT; default:D}}', { payload: { v: 'X' } })).toBe('HIT');
    expect(resolve('{{v:eq; X:HIT; default:D}}', { payload: { v: 'X' } })).toBe('HIT');
    expect(reports.map(({ code }) => code)).toEqual(['unknown-modifier']);
  });
  it('the modifier registry is read as own properties, never through a prototype', () => {
    const reports: Report[] = [];
    // A registry is a caller's object like the payload and the props layers,
    // and it is read the same way: what it inherits it does not hold. Types
    // describe a table's shape, never whose object the entries sit on.
    const inherited = Object.create({ 'x-inherited': () => 'INHERITED' });
    const { resolve } = createParser({ customModifiers: inherited, onReport: (entry) => reports.push(entry) });

    expect(resolve('{{v:x-inherited; default:D}}', { payload: { v: 'X' } })).toBe('D');
    // The prototype every object inherits from is the same story one step
    // further out: a name written there registers no modifier, and one written
    // over a built-in leaves the built-in answering.
    expect(polluted('x-polluted', () => 'HIJACKED', () => createParser({ customModifiers: {} }).resolve('{{v:x-polluted; default:D}}', { payload: { v: 'X' } }))).toBe('D');
    expect(polluted('number', () => 'HIJACKED', () => createParser({ customModifiers: {} }).resolve('{{v:number}}', { payload: { v: 1.5 }, locale: defaultLocale }))).toBe('1.5');
    expect(reports.map(({ code }) => code)).toEqual(['unknown-modifier']);
  });
  it('a modifier registered under a prototype name answers to it', () => {
    const reports: Report[] = [];
    // A registry reaches the parser from JavaScript, where a name a prototype
    // already answers to is a name like any other. The table is built onto a
    // null prototype and finished with a spread, which defines rather than
    // assigns, so the name stays the table's own.
    const registry: Parser.Options['customModifiers'] = { ['__proto__']: ({ value }) => `HIT:${value}` };
    const { resolve } = createParser({ customModifiers: registry, onReport: (entry) => reports.push(entry) });

    expect(resolve('{{v:__proto__; default:D}}', { payload: { v: 'X' } })).toBe('HIT:X');
    expect(reports).toHaveLength(0);
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
  it('a modifier the caller registers under a built-in name answers in its place', () => {
    const reports: Report[] = [];
    const { resolve } = createParser({
      customModifiers: { eq: ({ value }) => `CUSTOM:${value}`, number: () => 'CUSTOM NUMBER' },
      onReport: (report) => { reports.push(report); },
    });

    // A caller's table layers over the built-in one rather than under it, so a
    // name it carries is the modifier a message writing that name reaches —
    // the placeholder that names none and runs `eq` included.
    expect(resolve('{{v:eq; X:HIT; default:D}}', { payload: { v: 'X' } })).toBe('CUSTOM:X');
    expect(resolve('{{v; X:HIT; default:D}}', { payload: { v: 'X' } })).toBe('CUSTOM:X');
    expect(resolve('{{v:number}}', { payload: { v: 1.5 }, locale: defaultLocale })).toBe('CUSTOM NUMBER');

    expect(reports).toHaveLength(0);
  });
  it('a value that cannot become text is reported, and one nobody passed is not', () => {
    const reports: Report[] = [];
    const { resolve } = createParser({ onReport: (report) => { reports.push(report); } });

    expect(resolve('{{v; default:INLINE}}', { payload: { v: circular }, key: 'common.opaque' })).toBe('INLINE');

    expect(reports).toHaveLength(1);
    expect(reports[0]).toEqual({
      code: 'unserializable-value',
      origin: 'payload',
      message: 'A value could not become text, so resolution read it as missing.',
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
  it('a chain link whose read raises is reported like a value that could not become text', () => {
    const reports: Report[] = [];
    const { resolve } = createParser({ onReport: (report) => { reports.push(report); } });
    const raise = () => { throw new Error('NO READ'); };

    expect(resolve('{{v; default:INLINE}}', { payload: { get v() { return raise(); } } })).toBe('INLINE');
    expect(resolve('{{v; default:INLINE}}', { payload: { v: { get value() { return raise(); } } } })).toBe('INLINE');
    expect(resolve('{{v; default:INLINE}}', { payload: { v: { value: undefined, get default() { return raise(); } } } })).toBe('INLINE');
    expect(resolve('{{v; default:INLINE}}', { payload: { get default() { return raise(); } } })).toBe('INLINE');

    expect(reports.map(({ code }) => code)).toEqual(Array(4).fill('unserializable-value'));

    // A link that raises is read once, like any other link: the chain behind it
    // resolves from the report, not from a second read of the same entry.
    expect(resolve('{{default}}', { payload: { get default() { return raise(); } } })).toBe('');
    expect(resolve('{{v}}', { payload: { v: { get value() { return raise(); }, get default() { return raise(); } } } })).toBe('');

    expect(reports).toHaveLength(7);
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
  it('a modifier reads its default through an accessor, so the read is what walks the chain', () => {
    const reports: Report[] = [];
    const descriptors: (PropertyDescriptor | undefined)[] = [];
    const { resolve } = createParser({
      customModifiers: {
        'x-descriptor': (config) => { descriptors.push(Object.getOwnPropertyDescriptor(config, 'defaultValue')); return 'DONE'; },
        'x-named': ({ value }) => value,
        'x-copy': (config) => ({ ...config }).value,
      },
      onReport: (report) => { reports.push(report); },
    });
    const payload = { v: 'V', default: circular };

    expect(resolve('{{v:x-descriptor}}', { payload: { v: 'V', default: 'D' } })).toBe('DONE');
    expect(typeof descriptors[0]?.get).toBe('function');
    expect(descriptors[0]).toMatchObject({ enumerable: true, configurable: true });
    expect(descriptors[0]).not.toHaveProperty('value');

    // The chain is work the read does, so a modifier taking the keys it needs
    // by name never reaches the link nothing can describe, and one copying the
    // config generically reads the accessor and does.
    expect(resolve('{{v:x-named}}', { payload })).toBe('V');
    expect(reports).toHaveLength(0);

    expect(resolve('{{v:x-copy}}', { payload })).toBe('V');
    expect(reports.map(({ code }) => code)).toEqual(['unserializable-value']);
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
  it('a modifier that could not produce a result reports', () => {
    const reports: Report[] = [];
    const onReport = (entry: Report) => reports.push(entry);
    const raising = { 'x-raise': () => { throw new Error('MODIFIER FAILURE'); } };
    const { resolve } = createParser({ customModifiers: raising, onReport });

    // Containment is what keeps a modifier out of the caller's render path; it
    // is not a reason for the caller to hear nothing about it.
    expect(resolve('{{v:x-raise; default:D}}', { payload: { v: 'X' } })).toBe('D');
    expect(resolve('{{v:number; default:D}}', { payload: { v: 1 }, locale: 'not a locale' })).toBe('D');
    expect(resolve('{{v:currency; default:D}}', { payload: { v: 1 }, locale: defaultLocale })).toBe('D');
    expect(reports.map(({ code }) => code)).toEqual(['failed-modifier', 'failed-modifier', 'failed-modifier']);

    // The report names the placeholder that named the modifier, like the one a
    // name nobody registered earns. Its `message` is the text a host prints,
    // and it is the same sentence whatever the modifier raised: `text` is the
    // one field a report derives from what it was resolving.
    expect(reports[0].text).toBe('{{v:x-raise; default:D}}');
    expect(reports[0].message).toBe('A modifier could not produce a result, so the placeholder took its fallback chain.');

    // A modifier that answers, with nothing or with text, has produced a
    // result: the chain it may land on is the message's own answer, not a
    // failure.
    reports.length = 0;
    const answering = createParser({ customModifiers: { 'x-none': () => undefined, 'x-text': () => 'OK' }, onReport });

    expect(answering.resolve('{{v:x-none; default:D}}', { payload: { v: 'X' } })).toBe('D');
    expect(answering.resolve('{{v:x-text; default:D}}', { payload: { v: 'X' } })).toBe('OK');
    expect(reports).toEqual([]);
  });
  it('a formatting modifier resolves to the empty string where no locale is available', () => {
    const { resolve } = defaultParser;

    // A locale is not available where the caller supplied none and where what
    // it supplied is empty. A locale the caller did supply and the host then
    // rejects is available but unusable, which is a formatting failure and
    // takes the chain like any other.
    //
    // The locale is read before the value, so a value the modifier could not
    // have formatted answers the same way: a declared default stands in for a
    // value the modifier cannot read, never for a locale nobody supplied.
    for (const modifier of ['number', 'date', 'ago', 'currency']) {
      for (const value of [10, 'not a number']) {
        expect(resolve(`{{value:${modifier}; default:FALLBACK;}}`, { payload: { value } })).toBe('');
        expect(resolve(`{{value:${modifier}; default:FALLBACK;}}`, { payload: { value }, locale: '' })).toBe('');
      }
    }

    expect(resolve(message(defaultLocale, 'common.modifier_number_default'), { payload: { value: 10 } })).toBe('');
  });
  it('a formatting modifier given no locale reports', () => {
    const reports: Report[] = [];
    const seen = { 'x-locale': ({ locale }: { locale?: string }) => `[${locale ?? ''}]` };
    const { resolve } = createParser({ customModifiers: seen, onReport: (report) => { reports.push(report); } });

    const answer = (placeholder: string, context: Parser.Context) => {
      reports.length = 0;

      const text = resolve(placeholder, context);

      return { text, reported: reports.map(({ code, origin }) => `${code}/${origin}`) };
    };

    // The empty string is what the specification asks for, and the report is
    // what says why the placeholder came out empty. Its origin is the payload:
    // a locale nobody supplied is a defect in what the caller passed, not in
    // the message that was written.
    for (const modifier of ['number', 'date', 'ago', 'currency']) {
      for (const locale of [undefined, '']) {
        expect(answer(`{{v:${modifier}; default:FALLBACK}}`, { payload: { v: 10 }, locale })).toEqual({ text: '', reported: ['missing-locale/payload'] });
      }
    }

    // The locale is read before the value, so a value the modifier could not
    // have formatted either reports the locale and nothing else.
    expect(answer('{{v:number; default:FALLBACK}}', { payload: { v: 'not a number' } })).toEqual({ text: '', reported: ['missing-locale/payload'] });

    expect(answer('{{v:date}}', { payload: { v: 10 } })).toEqual({ text: '', reported: ['missing-locale/payload'] });
    expect(reports[0].message).toBe('A formatting modifier was given no locale, so the placeholder resolved to the empty string.');
    expect(reports[0].text).toBe('{{v:date}}');

    // A placeholder whose value is absent takes its chain before any modifier
    // is called, so there is no modifier there to be given a locale.
    expect(answer('{{v:number; default:FALLBACK}}', { payload: {} })).toEqual({ text: 'FALLBACK', reported: [] });
    expect(answer('{{v:number; default:FALLBACK}}', { payload: { v: undefined } })).toEqual({ text: 'FALLBACK', reported: [] });

    // A comparison reads no locale, and a host-defined modifier reads whatever
    // it likes: the report belongs to the four that format.
    expect(answer('{{v:eq; 10:TEN}}', { payload: { v: 10 } })).toEqual({ text: 'TEN', reported: [] });
    expect(answer('{{v; 10:TEN}}', { payload: { v: 10 } })).toEqual({ text: 'TEN', reported: [] });
    expect(answer('{{v:x-locale}}', { payload: { v: 10 } })).toEqual({ text: '[]', reported: [] });
  });
  it('a value that cannot become text resolves to the fallback chain', () => {
    const { resolve } = defaultParser;

    expect(resolve('{{value}}', { payload: { value: circular } })).toBe('');
    expect(resolve('{{value}}', { payload: { value: raisingRead, default: 'FALLBACK' } })).toBe('FALLBACK');
    expect(resolve('{{value}}', { payload: { value: noDescription, default: 'FALLBACK' } })).toBe('FALLBACK');
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

    // A caller that named neither has nothing left to fall back to, whichever
    // way the conversion failed: a read that raises and a value that serializes
    // to nothing are as absent as a message that raises on the way to text.
    expect(resolve(circular)).toBe('');
    expect(resolve(new Opaque())).toBe('');
    expect(resolve(raisingRead)).toBe('');
    expect(resolve(noDescription)).toBe('');

    // A message a conversion does describe is still the message.
    expect(resolve({ a: 1 }, { payload, key: 'common.key' })).toBe('{"a":1}');
    expect(resolve(42, { payload, key: 'common.key' })).toBe('42');
  });
  it('an empty message is a message, and the chain stops at it', () => {
    const { resolve } = defaultParser;

    // The chain steps past a message no conversion describes, not past one that
    // describes to nothing: a translator who wrote an empty string wrote a
    // message, and answering it with the payload's link or with the key would
    // put text on screen where that translator asked for none.
    expect(resolve('', { payload: { default: 'CHAIN' }, key: 'common.key' })).toBe('');
    expect(resolve('', { key: 'common.key' })).toBe('');
    expect(resolve('', { payload: { default: 'CHAIN' } })).toBe('');
  });
  it('the chain a message resolves through reports what it read and could not describe', () => {
    const reports: Report[] = [];
    const { resolve } = createParser({ onReport: (report) => { reports.push(report); } });

    // The message resolved, so nothing read the payload's link behind it.
    expect(resolve('MESSAGE', { payload: { default: circular }, key: 'common.key' })).toBe('MESSAGE');
    expect(reports).toHaveLength(0);

    // The message did not, so the link is read — and a link that is present
    // and cannot be described is a payload value that went missing, the same
    // one a placeholder would report. The report names no placeholder, because
    // there is none: the key is what says which message went looking.
    expect(resolve(circular, { payload: { default: circular }, key: 'common.key' })).toBe('common.key');
    expect(reports).toEqual([{
      code: 'unserializable-value',
      origin: 'payload',
      message: 'A value could not become text, so resolution read it as missing.',
      key: 'common.key',
      limit: undefined,
      text: '',
    }]);

    // A link that refuses to be read reports too, like one that cannot become
    // text: both are entries the payload carries and resolution could not use.
    expect(resolve(circular, { payload: { get default() { throw new Error('READ FAILURE'); } }, key: 'common.key' })).toBe('common.key');
    expect(reports.map(({ code }) => code)).toEqual(Array(2).fill('unserializable-value'));

    // A link nobody passed is not a defect, and neither is a message nothing
    // describes: that message is one nobody wrote, so it echoes its key alone.
    expect(resolve(circular, { payload: {}, key: 'common.key' })).toBe('common.key');
    expect(resolve(circular, { key: 'common.key' })).toBe('common.key');
    expect(reports).toHaveLength(2);
  });
  it('a message default no conversion can describe is skipped, not resolved', () => {
    const { resolve } = defaultParser;

    expect(resolve(undefined, { payload: { default: circular }, key: 'common.key' })).toBe('common.key');
    expect(resolve(undefined, { payload: { default: new Opaque() }, key: 'common.key' })).toBe('common.key');
    expect(resolve(undefined, { payload: { default: circular } })).toBe('');
  });
  it('the key echo is the key, not text the format resolves over', () => {
    const reports: Report[] = [];
    const { resolve } = createParser({ onReport: (report) => { reports.push(report); } });
    const backslash = String.fromCharCode(92);

    // A key echo is what the format says when there is no message, not one of
    // the things that may resolve to text carrying placeholders. A key shaped
    // like a placeholder is therefore echoed rather than resolved, and an
    // escape sequence inside one stays as the caller spelled it.
    expect(resolve(undefined, { payload: { name: 'Alice' }, key: '{{name}}' })).toBe('{{name}}');
    expect(resolve(undefined, { key: '{{lit}}' })).toBe('{{lit}}');
    expect(resolve(undefined, { key: `a${backslash};b` })).toBe(`a${backslash};b`);
    expect(reports).toHaveLength(0);

    // Nothing resolves, so no bound is reached and the payload behind the key
    // is not read a second time: the echo ends the chain.
    expect(resolve(undefined, { payload: { a: '{{a}}' }, key: '{{a}}' })).toBe('{{a}}');
    expect(resolve(undefined, { payload: { default: circular }, key: '{{name}}' })).toBe('{{name}}');
    expect(reports.map(({ code }) => code)).toEqual(['unserializable-value']);

    // The key still becomes text, because `resolve` answers with text, and a
    // caller that named no key still has nothing to echo.
    expect(resolve(undefined, { key: 42 as any })).toBe('42');
    expect(resolve(undefined, { key: {} as any })).toBe('{}');
    expect(resolve(undefined, {})).toBe('');
  });
  it('a message becomes text before it is read, not after', () => {
    const { resolve } = defaultParser;

    // A host's own string object carries a message like any other: its
    // placeholders resolve and its escape sequences are removed.
    expect(resolve(new String('{{value}}'), { payload: { value: 'TEST_STRING' } })).toBe('TEST_STRING');
    expect(resolve(new String('a\\;b'))).toBe('a;b');
    expect(resolve({ a: '{{value}}' }, { payload: { value: 'TEST_STRING' } })).toBe('{"a":"TEST_STRING"}');
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
      const { resolve } = createParser({ customModifiers });

      expect(resolve('{{value}}', { payload: { value: 'TEST_STRING' } })).toBe('TEST_STRING');
      expect(resolve('{{value:test}}', { payload: { value: 'TEST_STRING', default: 'FALLBACK' } })).toBe('FALLBACK');
    }
  });
  it('a custom modifier set to `undefined` leaves the modifier beneath it standing', () => {
    const { resolve } = createParser({ customModifiers: { eq: undefined } as unknown as Parser.Options['customModifiers'] });

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
  it('a formatting modifier that cannot format its input reports', () => {
    const reports: Report[] = [];
    const { resolve } = createParser({ onReport: (report) => { reports.push(report); } });
    const props = { currency: { currency: 'USD', ratio: 21.4 } };

    const answer = (placeholder: string, value: any) => {
      reports.length = 0;

      const text = resolve(placeholder, { payload: { v: value }, props, locale: defaultLocale });

      return { text, reported: reports.map(({ code, origin }) => `${code}/${origin}`) };
    };

    // Each of the four tests its value before the host's formatter sees it, and
    // a value that fails that test used to take the fallback chain in silence
    // where the same placeholder under a locale the host rejects reported. Both
    // halves of the failure report now, and the output is the one the modifier
    // read for itself: the same chain, resolved by the same read.
    for (const modifier of ['number', 'date', 'ago', 'currency']) {
      for (const value of ['not a number', '', '   ']) {
        expect(answer(`{{v:${modifier}; default:FALLBACK}}`, value)).toEqual({ text: 'FALLBACK', reported: ['failed-modifier/message'] });
      }

      expect(answer(`{{v:${modifier}}}`, 'not a number')).toEqual({ text: '', reported: ['failed-modifier/message'] });
    }

    // `currency` reads its value and then multiplies it by its ratio, so the
    // product is a second input it can reject.
    expect(answer('{{v:currency; default:FALLBACK}}', 1e308)).toEqual({ text: 'FALLBACK', reported: ['failed-modifier/message'] });
    expect(reports[0].text).toBe('{{v:currency; default:FALLBACK}}');
  });
  it('the formatting modifiers answer a value none of them can format alike', () => {
    const reports: Report[] = [];
    const { resolve } = createParser({ onReport: (report) => { reports.push(report); } });
    const props = { currency: { currency: 'USD' } };

    const answer = (modifier: string, value: any) => {
      reports.length = 0;

      const text = resolve(`{{v:${modifier}; default:FALLBACK}}`, { payload: { v: value }, props, locale: defaultLocale, key: 'common.key' });

      return `${text} ${reports.map(({ code }) => code).join(' ')}`;
    };

    // All four reject an input they cannot format before the host's formatter
    // sees it, so they answer such a value alike: the same text, and the same
    // nothing or something on the report channel. What is pinned is that they
    // agree, not what they agree on.
    for (const value of ['not a number', '   ', {}, [1, 2], true]) {
      for (const modifier of ['date', 'ago', 'currency']) {
        expect(answer(modifier, value)).toBe(answer('number', value));
      }
    }
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
  it('an infinite value is not a number a formatting modifier can format', () => {
    const { resolve } = createParser({});

    // The host's own conversion answers with infinity both for a value that
    // spells it and for a decimal that overflows into it, and neither is a
    // count a locale has a rendering for.
    for (const value of [Infinity, -Infinity, 'Infinity', '1e400', '-1e400']) {
      expect(resolve('{{v:number; default:FALLBACK}}', { payload: { v: value }, locale: defaultLocale })).toBe('FALLBACK');
    }
  });
  it('`currency` falls back where its ratio leaves no number to format', () => {
    const { resolve } = defaultParser;
    const at = (ratio: number, value: unknown = 2) => resolve('{{v:currency; default:FALLBACK}}', { payload: { v: value }, props: { currency: { currency: 'USD', ratio } }, locale: defaultLocale });

    // The value is read as a number before the ratio is applied, so the
    // product is a second place the pair stops being one: a ratio that is not
    // finite, a ratio that turns a finite amount into infinity, and the zero
    // times infinity that answers `NaN` all leave nothing to format.
    expect(at(1)).toBe(new Intl.NumberFormat(defaultLocale, { style: 'currency', currency: 'USD' }).format(2));
    expect(at(Infinity)).toBe('FALLBACK');
    expect(at(-Infinity)).toBe('FALLBACK');
    expect(at(NaN)).toBe('FALLBACK');
    expect(at(10, 1e308)).toBe('FALLBACK');
    expect(at(Infinity, 0)).toBe('FALLBACK');
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

    // Numeric text is read as a timestamp before the host's date grammar sees
    // it, and the two readings disagree wherever that grammar accepts a number
    // too: `2024` is 2024 milliseconds after the epoch, not the year of it.
    expect(Date.parse('2024')).toBe(Date.parse('2024-01-01T00:00:00.000Z'));
    expect(resolve('{{v:date; default:FALLBACK}}', { payload: { v: '2024' }, locale: defaultLocale })).toBe(new Intl.DateTimeFormat(defaultLocale, {}).format(2024));

    expect(resolve('{{v:date; default:FALLBACK}}', { payload: { v: 'tomorrow' }, locale: defaultLocale })).toBe('FALLBACK');
    expect(resolve('{{v:date; default:FALLBACK}}', { payload: { v: '' }, locale: defaultLocale })).toBe('FALLBACK');

    expect(resolve('{{v:number; default:FALLBACK}}', { payload: { v: '2024-03-05T10:00:00.000Z' }, locale: defaultLocale })).toBe('FALLBACK');
    expect(resolve('{{v:ago; default:FALLBACK}}', { payload: { v: '2024-03-05T10:00:00.000Z' }, locale: defaultLocale })).toBe('FALLBACK');
    expect(resolve('{{v:currency; default:FALLBACK}}', { payload: { v: '2024-03-05T10:00:00.000Z' }, props: { currency: { currency: 'USD' } }, locale: defaultLocale })).toBe('FALLBACK');
  });
  it('a date the host cannot parse answers nothing, not the host\'s own word for nothing', () => {
    // Both readings a timestamp can fail at answer the same way, because the
    // modifier tests for nothing and hands whatever it gets to `Intl`: an
    // answer of `NaN` reads as a timestamp the format would then have to raise
    // over, and containment is not what makes a value the modifier cannot
    // format take the fallback chain.
    expect(getModifierInput('tomorrow')).toBeUndefined();
    expect(getDateInput('tomorrow')).toBeUndefined();
    expect(getDateInput('')).toBeUndefined();
    expect(getDateInput('2024-13-45')).toBeUndefined();
    expect(getDateInput({})).toBeUndefined();

    const stamp = Date.parse('2024-03-05T10:00:00.000Z');

    expect(getDateInput(`${stamp}`)).toBe(stamp);
    expect(getDateInput('2024-03-05T10:00:00.000Z')).toBe(stamp);
    expect(getDateInput(0)).toBe(0);
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
  it('whitespace an escape sequence does not claim is still padding', () => {
    const { resolve } = defaultParser;

    // An escape sequence claims the one character behind the backslash. Blanks
    // past it are padding, and a trim that let the sequence claim them too
    // would carry them into a rendered message and into the key a placeholder
    // reads by.
    expect(resolve('{{v; 1:ONE\\: }}', { payload: { v: 1 } })).toBe('ONE:');
    expect(resolve('{{v; 1:ONE\\  }}', { payload: { v: 1 } })).toBe('ONE ');
    expect(resolve('{{v; default:D\\: }}', { payload: {} })).toBe('D:');
    expect(resolve('{{v\\: ; default:D}}', { payload: { 'v:': 'HIT' } })).toBe('HIT');
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
  it('the whitespace class is exactly what it enumerates, and nothing else in the plane', () => {
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
  it('the first segment named `default` is the inline default', () => {
    const { resolve } = defaultParser;

    expect(resolve('{{v; default:FIRST; default:SECOND}}')).toBe('FIRST');
    expect(resolve('{{v; default:; default:SECOND}}')).toBe('');
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
      origin: 'limit',
      message: 'Interpolation stopped after 10 passes. A payload value probably references its own placeholder.',
      key: 'common.placeholder_chain',
      limit: 10,
      text: '{{v11}}',
    });
  });
  it('a value whose serialization outgrows a resolvable output is read as missing', () => {
    const reports: Report[] = [];
    const { resolve } = createParser({ onReport: (report) => { reports.push(report); } });

    // Twenty-five objects, each of twenty-four levels naming the same child
    // twice. Nothing here is circular, so serialization has nothing to refuse —
    // it just walks sixteen million leaves to describe twenty-five objects.
    let shared: unknown = { leaf: 1 };

    for (let level = 0; level < 24; level += 1) shared = { a: shared, b: shared };

    expect(resolve('{{v; default:INLINE}}', { payload: { v: shared } })).toBe('INLINE');
    expect(resolve(shared as string, { payload: {}, key: 'common.shared' })).toBe('common.shared');

    expect(reports.map(({ code }) => code)).toEqual(['unserializable-value']);

    // The budget bounds the walk, not the value: a value that describes itself
    // in one pass is serialized however deep or wide it is.
    let chain: unknown = { leaf: 1 };

    for (let level = 0; level < 24; level += 1) chain = { a: chain };

    expect(resolve('{{v}}', { payload: { v: chain } })).toBe(`${'{"a":'.repeat(24)}{"leaf":1}${'}'.repeat(24)}`);
    expect(resolve('{{v}}', { payload: { v: Array.from({ length: 1000 }, (_, index) => index) } })).toBe(JSON.stringify(Array.from({ length: 1000 }, (_, index) => index)));

    expect(reports).toHaveLength(1);
  });
  it('the conversion budget is a node count a walk may reach, not one it may not', () => {
    const reports: Report[] = [];
    const { resolve } = createParser({ onReport: (report) => { reports.push(report); } });

    // The value itself is the first node the walk visits, so a bag of one key
    // fewer than the bound is walked exactly to it. Every entry is `undefined`,
    // which JSON omits, so what the walk costs is not what the text costs and
    // the output bound never sees either bag. The bound is spelled out here
    // rather than read from the source: a count derived from the constant it is
    // meant to pin would move along with it.
    const budget = 100000;
    const bag = (keys: number) => Object.fromEntries(Array.from({ length: keys }, (_, index) => [`k${index}`, undefined]));

    expect(resolve('{{v; default:INLINE}}', { payload: { v: bag(budget - 1) } })).toBe('{}');
    expect(resolve('{{v; default:INLINE}}', { payload: { v: bag(budget) } })).toBe('INLINE');

    expect(reports.map(({ code }) => code)).toEqual(['unserializable-value']);
  });
  it('a value converts once, however many reads a resolution makes of it', () => {
    const { resolve } = defaultParser;

    let conversions = 0;
    // An enumerable getter is read once per walk, so the count is the number of
    // conversions the resolution made of the object carrying it.
    const counted = (json: string) => ({ get read() { conversions += 1; return json; } });

    expect(resolve('{{v}}'.repeat(50), { payload: { v: counted('A') } })).toBe('{"read":"A"}'.repeat(50));
    expect(conversions).toBe(1);

    // A value whose own text carries the placeholder that read it is read again
    // on every pass and twice as often on each, so the ten passes reach this one
    // 1023 times.
    conversions = 0;

    resolve('{{v}}', { payload: { v: counted('{{v}}{{v}}') } });

    expect(conversions).toBe(1);

    // Identity is what a conversion answers for. Two values that describe
    // themselves alike are still two values.
    conversions = 0;

    resolve('{{v}}{{w}}', { payload: { v: counted('A'), w: counted('A') } });

    expect(conversions).toBe(2);

    // The ordinary string conversion is a conversion too. A value the host
    // describes through its own `toString` answers every later read with the
    // text the first one produced, so host code a value carries runs once for
    // the resolution rather than once for each placeholder naming it.
    let coercions = 0;

    class Coerced {
      toString() { coercions += 1; return 'T'; }
    }

    expect(resolve('{{v}}'.repeat(50), { payload: { v: new Coerced() } })).toBe('T'.repeat(50));
    expect(coercions).toBe(1);

    // The answer that no conversion describes it is recorded alike: a
    // `toString` that raises is not asked a second time, so one value cannot be
    // absent at one placeholder and present at the next.
    coercions = 0;

    class Raising {
      toString(): string { coercions += 1; throw new Error('undescribable'); }
    }

    expect(resolve('{{v}}|{{v}}', { payload: { v: new Raising(), default: 'D' } })).toBe('D|D');
    expect(coercions).toBe(1);

    // A function carries host code the same way an object does, so it is
    // recorded the same way: it is a value the format converts, not a
    // primitive whose conversion nobody can observe.
    coercions = 0;

    const carrier = () => 'UNCALLED';

    carrier.toString = () => { coercions += 1; return 'FN'; };

    expect(resolve('{{v}}{{v}}{{v}}', { payload: { v: carrier } })).toBe('FNFNFN');
    expect(coercions).toBe(1);
  });
  it('a conversion is recorded for the call that made it, not for the parser', () => {
    const { resolve } = defaultParser;

    let coercions = 0;

    class Counted {
      toString() { coercions += 1; return `T${coercions}`; }
    }

    const value = new Counted();

    // One call is the scope. Inside it the first read answers every later one;
    // the next call converts afresh, however many calls the parser has served.
    expect(resolve('{{v}}{{v}}', { payload: { v: value } })).toBe('T1T1');
    expect(resolve('{{v}}{{v}}', { payload: { v: value } })).toBe('T2T2');
    expect(coercions).toBe(2);

    // So a payload the host mutates between two calls is answered with what it
    // says now, not with the text an earlier call came out with.
    const mutated = { a: 1 };

    expect(resolve('{{v}}', { payload: { v: mutated } })).toBe('{"a":1}');

    mutated.a = 2;

    expect(resolve('{{v}}', { payload: { v: mutated } })).toBe('{"a":2}');
  });
  it('a resolution begun inside another is its own scope', () => {
    const reports: Report[] = [];
    const nested: string[] = [];

    let coercions = 0;

    class Counted {
      toString() { coercions += 1; return 'C'; }
    }

    const counted = new Counted();
    const big = 'x'.repeat(60000);
    // A modifier resolving a message of its own is the plainest way a host
    // begins a resolution inside one; an `onReport` handler writing its
    // diagnostic into a translated string, a payload accessor and a value's
    // own `toString` all reach it too.
    const parser: Parser.T = createParser({
      onReport: (report) => { reports.push(report); },
      customModifiers: {
        'x-nest': ({ value }) => {
          nested.push(parser.resolve(value, { payload: { c: counted, loop: '{{loop}}', big }, key: 'inner' }));

          return `[${nested.length}]`;
        },
      },
    });

    // The call is the scope, and a call begun inside another is a call: each
    // converts the value they share once for itself, counts its own passes and
    // spends its own output budget, and neither reaches a bound the other
    // owns. The inner call stops at ten passes while the outer walks a
    // three-link chain to the end, and stops at the output bound while the
    // outer goes on building the text it was building.
    expect(parser.resolve('{{c}} {{m:x-nest}} {{a}}', { payload: { c: counted, m: '{{c}}{{c}} {{loop}}', a: '{{b}}', b: '{{d}}', d: 'D' }, key: 'outer' })).toBe('C [1] D');
    expect(parser.resolve('{{m:x-nest}}{{big}}', { payload: { m: '{{big}}{{big}}', big }, key: 'outer' })).toBe(`[2]${big}`);

    expect(nested).toEqual(['CC {{loop}}', '{{big}}{{big}}']);
    expect(coercions).toBe(2);

    // And a report names the message the call that made it was resolving, not
    // the one further out.
    expect(reports.map(({ code, key }) => `${key}/${code}`)).toEqual(['inner/pass-limit', 'inner/output-limit']);
  });
  it('a resolution that never stops beginning another still fails soft', () => {
    // Nothing bounds a resolution a host's own callback begins, so what ends
    // one that never stops beginning another is the host's own stack. Running
    // out of it is contained where every other failure is: the placeholder
    // takes the fallback chain and `resolve` still answers with text. Each
    // entry point gets a parser of its own — two of them feeding each other is
    // the host's own loop, not a bound resolution can hold.
    const modifier: Parser.T = createParser({ customModifiers: { 'x-loop': ({ value }) => modifier.resolve('{{v:x-loop}}', { payload: { v: value } }) } });
    const reporter: Parser.T = createParser({ onReport: () => { reporter.resolve('{{v:nope}}', { payload: { v: 'V' } }); } });
    const reader: Parser.T = createParser({});
    const coercer: Parser.T = createParser({});
    const payload = { get v(): string { return reader.resolve('{{v}}', { payload }); } };

    class Coerced {
      toString(): string { return coercer.resolve('{{v}}', { payload: { v: new Coerced() } }); }
    }

    expect(modifier.resolve('{{v:x-loop}}!', { payload: { v: 'V' } })).toBe('!');
    expect(reporter.resolve('{{v:nope}}!', { payload: { v: 'V' } })).toBe('!');
    expect(reader.resolve('{{v}}!', { payload })).toBe('!');
    expect(coercer.resolve('{{v}}!', { payload: { v: new Coerced() } })).toBe('!');
  });
  it('converting a value once leaves what a message resolves to unchanged', () => {
    const reports: Report[] = [];
    const { resolve } = createParser({ customModifiers: { test: ({ value }) => value }, onReport: (report) => { reports.push(report); } });

    const shared = { a: [1, 2] };

    expect(resolve('{{v}} {{w}} {{v:test}}', { payload: { v: shared, w: shared } })).toBe('{"a":[1,2]} {"a":[1,2]} {"a":[1,2]}');
    expect(resolve('{{v}}', { payload: { v: shared, default: shared } })).toBe('{"a":[1,2]}');
    expect(reports).toHaveLength(0);

    // A value that cannot become text is a defect wherever it is read, so the
    // recorded answer spares the walk and not the report.
    expect(resolve('{{v}} {{v}}', { payload: { v: circular } })).toBe(' ');
    expect(reports.map(({ code }) => code)).toEqual(Array(2).fill('unserializable-value'));
  });
  it('a reported excerpt is cut at the bound, not at whatever reached it', () => {
    const reports: Report[] = [];
    const { resolve } = createParser({ onReport: (report) => { reports.push(report); } });

    // Ten passes each prepend the fill once, so the output the guard reports on
    // is as long as the fill says. The bound is spelled out here rather than
    // read from the source: a length derived from the constant it is meant to
    // pin would move along with it.
    const limit = 120;
    const grow = (fill: string) => {
      reports.length = 0;
      resolve('{{a}}', { payload: { a: `${fill}{{a}}` }, locale: defaultLocale });

      return reports[0].text;
    };

    expect(grow('x'.repeat(11))).toBe(`${'x'.repeat(110)}{{a}}`);
    expect(grow('x'.repeat(13))).toBe(`${'x'.repeat(limit)}...`);
  });
  it('the report bound is a length an excerpt may reach, not one it may not', () => {
    const reports: Report[] = [];
    const { resolve } = createParser({ onReport: (report) => { reports.push(report); } });

    // A placeholder naming a modifier nobody registered reports the placeholder
    // itself, which is the one excerpt whose length a message spells exactly.
    // The bound is spelled out here rather than read from the source.
    const limit = 120;
    const named = (name: string) => {
      reports.length = 0;
      resolve(`{{v:${name}}}`, { payload: { v: 1 }, locale: defaultLocale });

      return reports[0].text;
    };
    const name = 'x'.repeat(limit - '{{v:}}'.length);
    // One character longer, the cut takes the first 120 and stops inside the
    // closing pair, so what the marker is appended to ends at a single brace.
    const cut = `{{v:${'x'.repeat(limit - '{{v:}'.length)}}`;

    expect(named(name)).toBe(`{{v:${name}}}`);
    expect(named(`${name}x`)).toBe(`${cut}...`);
  });
  it('the cut counts what reached the report, and the escaping happens after it', () => {
    const reports: Report[] = [];
    const { resolve } = createParser({ onReport: (report) => { reports.push(report); } });

    const limit = 120;
    // A placeholder naming a modifier nobody registered reports the placeholder
    // itself, so the excerpt is exactly what the message spelled.
    const named = (name: string) => {
      reports.length = 0;
      resolve(`{{v:${name}}}`, { payload: { v: 1 }, locale: defaultLocale });

      return reports[0].text;
    };

    expect(named('x'.repeat(200))).toBe(`{{v:${'x'.repeat(limit - 4)}...`);

    // Every backslash the cut carried leaves as two, so what arrives is longer
    // than the bound. Cutting the escaped form instead would hold the bound and
    // sever a sequence at it, which is the worse trade: the bound is there to
    // keep a report small, and the escaping is there to keep it safe.
    expect(named('\\'.repeat(200))).toBe(`{{v:${'\\\\'.repeat(limit - 4)}...`);
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
  it('a report escapes every terminator its excerpt carries, not only the first', () => {
    const reports: Report[] = [];
    const resolve = resolverFor<{ v1?: string }>(defaultLocale, createParser({ onReport: (report) => { reports.push(report); } }));

    // `JSON.stringify` writes a short escape for two of the four terminators
    // and leaves U+2028 and U+2029 raw, so an excerpt carrying more than one of
    // those is what an escaping pass stopping at its first match lets through.
    const run = LINE_TERMINATORS.join('');
    const output = resolve('common.placeholder_chain', { v1: `{{v1}}${run}` });

    expect(reports).toHaveLength(1);
    // Short of the cut, so the excerpt carries every terminator the output did.
    expect(output.length).toBeLessThanOrEqual(120);

    for (const terminator of LINE_TERMINATORS) {
      expect(output.split(terminator)).toHaveLength(11);
      expect(reports[0].text).not.toContain(terminator);
    }

    // The two `JSON.stringify` leaves raw are the ones this pass rewrites, and
    // it rewrites each of them.
    expect(reports[0].text.split('\\u2028')).toHaveLength(11);
    expect(reports[0].text.split('\\u2029')).toHaveLength(11);
  });
  it('exceeding the output budget stops interpolation and reports it', () => {
    const reports: Report[] = [];
    const resolve = resolverFor<{ v1?: string }>(defaultLocale, createParser({ onReport: (report) => { reports.push(report); } }));

    const output = resolve('common.placeholder_chain', { v1: `${'{{v1}}'.repeat(4)}${'x'.repeat(64)}` });

    expect(output.length).toBeLessThanOrEqual(100000);
    expect(output.length).toBe(27968);

    expect(reports).toHaveLength(1);
    expect(reports[0]).toMatchObject({
      code: 'output-limit',
      origin: 'limit',
      message: 'Interpolation stopped before exceeding 100000 characters. A payload value probably multiplies its own placeholder.',
      limit: 100000,
      key: 'common.placeholder_chain',
    });
    expect(reports[0].text.length).toBeLessThan(300);
  });
  it('the output budget is a length a pass may reach, not one it may not', () => {
    const reports: Report[] = [];
    const { resolve } = createParser({ onReport: (report) => { reports.push(report); } });
    // The bound is spelled out here rather than read from the source: a length
    // derived from the constant it is meant to pin would move along with it.
    const limit = 100000;
    const at = (length: number) => {
      reports.length = 0;

      return resolve('{{v}}', { payload: { v: 'x'.repeat(length) }, key: 'common.key' });
    };

    // A pass that lands exactly on the bound has not exceeded it, so what it
    // built is what resolves; one character further is a pass discarded whole,
    // and the last output under the bound is the message as it arrived.
    expect(at(limit)).toHaveLength(limit);
    expect(reports).toHaveLength(0);

    expect(at(limit + 1)).toBe('{{v}}');
    expect(reports.map(({ code }) => code)).toEqual(['output-limit']);
  });
  it('a pass is bounded as it is built, so it cannot outgrow what a string can hold', () => {
    const reports: Report[] = [];
    const { resolve } = createParser({ onReport: (report) => { reports.push(report); } });

    const multiplied = '{{b}}'.repeat(10400);
    const output = resolve('{{a}}', { payload: { a: multiplied, b: 'x'.repeat(52000) }, key: 'common.placeholder_chain' });

    expect(output).toBe(multiplied);

    expect(reports).toHaveLength(1);
    expect(reports[0]).toMatchObject({ code: 'output-limit', origin: 'limit', limit: 100000, key: 'common.placeholder_chain' });
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
      const payload = { value: `{{a}}{{${' '.repeat(size)}`, a: 'A' };

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
