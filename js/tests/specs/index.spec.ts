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

    expect(resolve('common.placeholder_default')).toBe('VALUES: DEFAULT_VALUE, DEFAULT_VALUE, DEFAULT_VALUE , DEFAULT_VALUE');
  });
  it('dynamic default works for placeholders', () => {
    const resolve = resolverFor<{ value?: any }>(defaultLocale);

    expect(resolve('common.placeholder_unknown', { default: 'DYNAMIC_DEFAULT_VALUE' })).toBe('DYNAMIC_DEFAULT_VALUE');
  });
  it('placeholders containing escaped values work', () => {
    const resolve = resolverFor<{ 'pl:ace;holder'?: any }>(defaultLocale);

    expect(resolve('common.placeholder_escaped', { 'pl:ace;holder': 'TEST \\{\\{VALUE\\}\\}' })).toBe('TEST {{VALUE}}');
  });
  it('`eq` modifier works', () => {
    const resolve = resolverFor<{ value?: any }>(defaultLocale);

    expect(resolve('common.modifier_eq', { value: 'option9' })).toBe('VALUES: DEFAULT VALUE, DEFAULT VALUE , DEFAULT VALUE, DEFAULT VALUE  ');
    expect(resolve('common.modifier_eq', { value: 'option2' })).toBe('VALUES: VALUE2, VALUE2 , VALUE2, VALUE2  ');
    expect(resolve('common.modifier_eq')).toBe('VALUES: DEFAULT VALUE, DEFAULT VALUE , DEFAULT VALUE, DEFAULT VALUE  ');
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
    expect(resolve('common.placeholder_default_single_char')).toBe('VALUES: a, a, a , a');
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
    expect(resolve('common.modifier_short_option', { value: 'x' })).toBe('VALUES: x, DEF, z');
    expect(resolve('common.modifier_short_option', { value: 5 })).toBe('VALUES: FIVE, 1, z');
    expect(resolve('common.modifier_short_option', { value: 2 })).toBe('VALUES: DEF, 1, z');
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
    expect(resolve('common.placeholder_inherited', { default: 'DEFAULT VALUE' })).toBe('VALUES: DEFAULT VALUE, DEFAULT VALUE, DEFAULT VALUE, INLINE DEFAULT');
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
