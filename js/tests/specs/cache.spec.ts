import { describe, expect, it } from 'vitest';
import { plan } from '@curly-message/conformance';
import type { Adapter } from '@curly-message/conformance';
import { createParser, Modifier, Parser, Report } from '@curly-message/parser';
import { calls, READS } from '../calls';
import { adapter, contextOf, parserFor } from '../conformance/adapter';

// Whether a resolution read the message's text: a message with a placeholder
// or an escape is read one code unit at a time, and one a parser kept is not
// read again.
const reads = (resolve: Parser.T['resolve'], message: string) => calls(READS, () => resolve(message, { payload: { a: 'A' } })) > 0;

// Messages with a placeholder each, distinct from `from` on, padded to `length`.
const distinct = (count: number, from = 0, length = 0) => Array.from({ length: count }, (_, index) => `${from + index}{{a}}`.padEnd(length, '.'));

// Seen twice, so a parser keeps it.
const keep = (resolve: Parser.T['resolve'], messages: string[]) => {
  for (const message of messages) {
    resolve(message, {});
    resolve(message, {});
  }
};

describe('what a parser keeps of a message', () => {
  it('reads a message seen twice before no more', () => {
    const { resolve } = createParser();

    expect(reads(resolve, 'Hello, {{a}}!')).toBe(true);
    expect(reads(resolve, 'Hello, {{a}}!')).toBe(true);
    expect(reads(resolve, 'Hello, {{a}}!')).toBe(false);
    expect(resolve('Hello, {{a}}!', { payload: { a: 'Ada' } })).toBe('Hello, Ada!');
  });
  it('keeps 1 024 messages, the oldest making room for the next', () => {
    const { resolve } = createParser();
    const [first, ...rest] = distinct(1025);

    keep(resolve, [first, ...rest.slice(0, 1023)]);
    expect(reads(resolve, first)).toBe(false);
    keep(resolve, rest.slice(1023));
    expect(reads(resolve, first)).toBe(true);
  });
  it('keeps a message no longer for being resolved again', () => {
    const { resolve } = createParser();
    const [first, second, ...rest] = distinct(1025);

    keep(resolve, [first, second, ...rest.slice(0, 1022)]);
    resolve(first, {});
    keep(resolve, rest.slice(1022));
    expect(reads(resolve, second)).toBe(false);
    expect(reads(resolve, first)).toBe(true);
  });
  it('keeps 131 072 characters of messages, the oldest making room', () => {
    // 15 messages of 8 192 characters, one of `filler`, and one of 8: kept
    // whole while that comes to the bound and no further.
    const firstKept = (filler: number) => {
      const { resolve } = createParser();
      const [first, ...rest] = distinct(15, 0, 8192);

      keep(resolve, [first, ...rest, ...distinct(1, 100, filler), ...distinct(1, 200, 8)]);

      return !reads(resolve, first);
    };

    expect(firstKept(8184)).toBe(true);
    expect(firstKept(8185)).toBe(false);
  });
  it('reads a message longer than 8 192 characters on every call, and makes no room for it', () => {
    const { resolve } = createParser();
    const kept = distinct(16, 0, 8192);
    const [long] = distinct(1, 100, 8193);

    keep(resolve, [...kept, long]);
    expect(reads(resolve, long)).toBe(true);
    expect(kept.map((message) => reads(resolve, message))).toEqual(kept.map(() => false));
  });
  it('makes no room for a message holding neither a placeholder nor an escape, however often it comes', () => {
    const { resolve } = createParser();
    const kept = distinct(1024);
    const plain = Array.from({ length: 2048 }, (_, index) => `plain ${index} {a} }}`);

    keep(resolve, kept);
    keep(resolve, plain);
    expect(plain.slice(0, 3).map((message) => resolve(message, {}))).toEqual(['plain 0 {a} }}', 'plain 1 {a} }}', 'plain 2 {a} }}']);
    expect(kept.map((message) => reads(resolve, message))).toEqual(kept.map(() => false));
  });
  it('keeps a message holding an escape and no placeholder as it keeps one with a placeholder', () => {
    const { resolve } = createParser();

    expect([0, 1, 2].map(() => reads(resolve, 'C:\\Users\\Ada \\{a\\}'))).toEqual([true, true, false]);
    expect(resolve('C:\\Users\\Ada \\{a\\}', {})).toBe('C:\\Users\\Ada {a}');
  });
  it('remembers a message seen once while 1 023 others are seen once, and not past them', () => {
    const remembered = (others: number) => {
      const { resolve } = createParser();
      const [message] = distinct(1);

      resolve(message, {});
      distinct(others, 1).forEach((other) => resolve(other, {}));
      resolve(message, {});

      return !reads(resolve, message);
    };

    expect(remembered(1023)).toBe(true);
    expect(remembered(1024)).toBe(false);
  });
  it('keeps messages seen twice while a stream of others is seen once', () => {
    const { resolve } = createParser();
    const kept = distinct(512);

    keep(resolve, kept);
    distinct(4096, 1000).forEach((other) => resolve(other, {}));
    expect(kept.map((message) => reads(resolve, message))).toEqual(kept.map(() => false));
  });
  it('shares nothing between two parsers', () => {
    const one = createParser();
    const other = createParser();

    keep(one.resolve, ['Hello, {{a}}!']);
    expect(reads(other.resolve, 'Hello, {{a}}!')).toBe(true);
    keep(other.resolve, distinct(2048));
    expect(reads(one.resolve, 'Hello, {{a}}!')).toBe(false);
  });
  it('resolves a kept message by what each call passes', () => {
    const reports: Report[] = [];
    const { resolve } = createParser({ onReport: (report) => { reports.push(report); } });
    const message = '{{a}} {{n:number}} {{n:plural; one:file; other:files}} {{b:nope}}';

    keep(resolve, [message]);
    reports.length = 0;
    expect(resolve(message, { payload: { a: 'A', n: 1 }, locale: 'en', id: 'first' })).toBe('A 1 file ');
    expect(resolve(message, { payload: { a: 'B', n: 2000 }, locale: 'de', id: 'second' })).toBe(`B ${new Intl.NumberFormat('de').format(2000)} files `);
    expect(reports.map(({ code, id }) => [code, id])).toEqual([['unknown-modifier', 'first'], ['unknown-modifier', 'second']]);
  });
  it('resolves a kept message by the modifiers of the parser that resolves it', () => {
    const custom: Modifier.T = ({ value }) => `<${value}>`;
    const plain = createParser();
    const extended = createParser({ customModifiers: { custom } });

    keep(plain.resolve, ['{{a:custom}}']);
    keep(extended.resolve, ['{{a:custom}}']);
    expect(plain.resolve('{{a:custom}}', { payload: { a: 'A' } })).toBe('');
    expect(extended.resolve('{{a:custom}}', { payload: { a: 'A' } })).toBe('<A>');
  });
  it('reads the payload of a kept message on every call, a getter that raises included', () => {
    const reports: Report[] = [];
    const { resolve } = createParser({ onReport: (report) => { reports.push(report); } });
    let raises = false;
    const payload = { get a() { if (raises) throw new Error('refused'); return 'A'; } };

    keep(resolve, ['{{a}}!']);
    expect(resolve('{{a}}!', { payload })).toBe('A!');
    raises = true;
    expect(resolve('{{a}}!', { payload })).toBe('!');
    expect(reports.map(({ code }) => code)).toEqual(['unserializable-value']);
  });
});

// Every case of the set resolved twice by one parser, the second time from
// what the first kept, and once more from it: the outputs and the reports
// agree with the first.
const thrice: Adapter = {
  ...adapter,
  resolve: (input) => {
    const reports: Report[] = [];
    const { resolve } = parserFor(input, reports);
    const runs = [0, 1, 2].map(() => ({ output: resolve(input.message, contextOf(input)), reports: reports.splice(0) }));

    expect(runs[1]).toEqual(runs[0]);
    expect(runs[2]).toEqual(runs[0]);

    return runs[2];
  },
};

// The cases that configure no modifier, all through one parser, so each finds
// what the cases before it left kept, a span half rendered included.
const reports: Report[] = [];
const shared = parserFor({}, reports);
const together: Adapter = {
  ...adapter,
  resolve: (input) => {
    if (input.modifiers || input.defaults) return adapter.resolve(input);

    reports.length = 0;

    const output = shared.resolve(input.message, contextOf(input));

    return { output, reports: [...reports] };
  },
};

describe('conformance through what a parser keeps', () => {
  for (const [name, runner] of [['thrice', thrice], ['together', together]] as const) {
    for (const planned of plan(runner).cases) {
      it(`${name}: ${planned.id}`, () => {
        expect(planned.execute()).toEqual({ ok: true });
      });
    }
  }
});
