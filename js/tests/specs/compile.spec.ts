import { describe, expect, it } from 'vitest';
import { plan } from '@curly-message/conformance';
import type { Adapter } from '@curly-message/conformance';
import { createParser, Parser, Report } from '@curly-message/parser';
import { calls, READS } from '../calls';
import { adapter, contextOf, parserFor } from '../conformance/adapter';

type Input = Parameters<Adapter['resolve']>[0];

// What `resolve` returns and reports for the message and the context, through
// a parser of the case's own that has resolved nothing before.
const resolved = (input: Input, context: Parser.Context) => {
  const reports: Report[] = [];

  return { output: parserFor(input, reports).resolve(input.message, context), reports };
};

// Every case of the set compiled once, and its compiled message resolved
// first with the context of the case before it, then twice with its own:
// each resolution returns and reports what `resolve` does for the message
// and that context.
let previous: Input | undefined;

const compiled: Adapter = {
  ...adapter,
  resolve: (input) => {
    const reports: Report[] = [];
    const { compile } = parserFor(input, reports);
    const message = compile(input.message);
    const resolution = (context: Parser.Context) => ({ output: message.resolve(context), reports: reports.splice(0) });

    if (previous) expect(resolution(contextOf(previous))).toEqual(resolved(input, contextOf(previous)));

    previous = input;

    const runs = [0, 1].map(() => resolution(contextOf(input)));

    expect(runs[0]).toEqual(resolved(input, contextOf(input)));
    expect(runs[1]).toEqual(runs[0]);

    return runs[1];
  },
};

describe('conformance through a compiled message', () => {
  for (const planned of plan(compiled).cases) {
    it(planned.id, () => {
      expect(planned.execute()).toEqual({ ok: true });
    });
  }
});

// A target whose every read is written to `log` under `name`.
const logged = <T extends object>(name: string, target: T, log: string[]) => new Proxy(target, {
  get: (object, key, receiver) => {
    log.push(`${name}.get ${String(key)}`);

    return Reflect.get(object, key, receiver);
  },
  has: (object, key) => {
    log.push(`${name}.has ${String(key)}`);

    return Reflect.has(object, key);
  },
  getOwnPropertyDescriptor: (object, key) => {
    log.push(`${name}.own ${String(key)}`);

    return Reflect.getOwnPropertyDescriptor(object, key);
  },
  ownKeys: (object) => {
    log.push(`${name}.keys`);

    return Reflect.ownKeys(object);
  },
  getPrototypeOf: (object) => {
    log.push(`${name}.prototype`);

    return Reflect.getPrototypeOf(object);
  },
});

// A message that is not text, and says how often it was asked to become text.
class Message {
  conversions = 0;

  constructor(private readonly text: string) {}

  toString() {
    this.conversions += 1;

    return this.text;
  }
}

describe('a compiled message', () => {
  it('reads the context and the options as resolve does, and compile reads neither', () => {
    const log: string[] = [];
    const options = logged('options', {
      customModifiers: { upper: ({ value }: { value: string }) => value.toUpperCase() },
      modifierDefaults: { number: { maximumFractionDigits: 1 } },
      onReport: (report: Report) => { log.push(`report ${report.code}`); },
    }, log);
    const { compile, resolve } = createParser(options);
    const text = '{{name:upper}} has {{count:number}} {{count:plural; one:file; other:files}}{{gone:nope}}.';
    const message = new Message(text);
    const context = () => logged('context', {
      payload: logged('payload', { name: 'Ada', count: 1234.56, gone: 'x' }, log),
      locale: 'en',
      id: 'files',
    }, log);

    const greeting = compile(message);

    expect([log.splice(0), message.conversions]).toEqual([[], 1]);

    const outputs = [greeting.resolve(context()), greeting.resolve(context())];
    const fromCompiled = log.splice(0);

    expect(message.conversions).toBe(1);

    const expected = [resolve(text, context()), resolve(text, context())];
    const fromText = log.splice(0);

    expect(outputs).toEqual(expected);
    expect(outputs[0]).toBe('ADA has 1,234.6 files.');
    expect(fromCompiled).toEqual(fromText);
    expect(fromCompiled).toContain('report unknown-modifier');

    resolve(message, context());

    expect(log.splice(0)).toEqual(fromText.slice(0, fromText.length / 2));
    expect(message.conversions).toBe(2);
  });

  it('resolves to what the text it compiled resolves to, whatever the message is now', () => {
    class Labelled {
      constructor(public label: string) {}

      toString() {
        return `{{x}}/${this.label}`;
      }
    }

    const { compile, resolve } = createParser();
    const message = new Labelled('a');
    const labelled = compile(message);

    message.label = 'b';

    expect(labelled.resolve({ payload: { x: message } })).toBe(resolve('{{x}}/a', { payload: { x: message } }));
    expect(labelled.resolve({ payload: { x: message } })).toBe('{{x}}/b/a');
    expect(resolve(message, { payload: { x: message } })).toBe('{{x}}/b/b');
  });

  it('reads a payload the caller changed between two resolutions afresh', () => {
    const { compile } = createParser();
    const user: Record<string, string> = { name: 'A' };
    const entry: Record<string, string> = { value: 'x' };
    const payload = { user, entry };
    const compiled = compile('{{user}} {{entry}}');

    expect(compiled.resolve({ payload })).toBe('{"name":"A"} x');
    user.name = 'B';
    entry.unit = 'kg';
    expect(compiled.resolve({ payload })).toBe('{"name":"B"} {"value":"x","unit":"kg"}');
  });

  it('compiles a message no conversion describes to one resolving to nothing, as resolve resolves it', () => {
    class Refusing {
      toString(): string {
        throw new Error('refused');
      }
    }

    const { proxy, revoke } = Proxy.revocable({}, {});

    revoke();

    for (const message of [new Refusing(), proxy, undefined]) {
      const reports: Report[] = [];
      const { compile, resolve } = createParser({ onReport: (report) => { reports.push(report); } });
      const context = { id: 'refused', get props(): never { throw new Error('refused'); } };
      const nothing = compile(message);

      expect(reports).toEqual([]);
      expect([nothing.resolve(context), reports.splice(0)]).toEqual([resolve(message, context), reports.splice(0)]);
      expect(nothing.resolve()).toBe('');
    }
  });

  it('converts a message that is not text as resolve converts it', () => {
    const { compile, resolve } = createParser();
    const messages = [5, 0, 10n, null, false, true, '', ['{{a}}', 1], { a: '{{a}}' }, new Date(0), Symbol('s')];

    expect(messages.map((message) => compile(message).resolve({ payload: { a: 'A' } }))).toEqual(messages.map((message) => resolve(message, { payload: { a: 'A' } })));
  });

  it('reports the text it compiled from, where the message was text', () => {
    const reports: Report[] = [];
    const { compile, resolve } = createParser({ onReport: (report) => { reports.push(report); } });
    const context = { get payload(): never { throw new Error('refused'); } };

    compile('{{a}}!').resolve(context);
    compile(new Message('{{a}}!')).resolve(context);
    resolve('{{a}}!', context);
    resolve(new Message('{{a}}!'), context);

    expect(reports.map(({ code, text }) => [code, text])).toEqual([
      ['unserializable-value', '{{a}}!'],
      ['unserializable-value', ''],
      ['unserializable-value', '{{a}}!'],
      ['unserializable-value', ''],
    ]);
  });

  it('resolves called apart from the parser and the compiled message', () => {
    const { compile } = createParser();
    const { resolve } = compile('a{{v}}');

    expect(resolve({ payload: { v: 1 } })).toBe('a1');
  });

  it('compiles and resolves inside a modifier of the parser compiling it', () => {
    const parser = createParser({
      customModifiers: { inner: ({ value }) => parser.compile('[{{v}}]').resolve({ payload: { v: value } }) },
    });

    expect(parser.compile('{{v:inner}}').resolve({ payload: { v: 'a' } })).toBe('[a]');
    expect(parser.resolve('{{v:inner}}', { payload: { v: 'a' } })).toBe('[a]');
  });

  it('reads its text on its first resolution and not again', () => {
    const { compile } = createParser();
    const greeting = compile('Hello, {{a}}!');
    const reads = () => calls(READS, () => greeting.resolve({ payload: { a: 'A' } }));

    expect(reads()).toBeGreaterThan(0);
    expect(reads()).toBe(0);
    expect(greeting.resolve({ payload: { a: 'Ada' } })).toBe('Hello, Ada!');
  });

  it('keeps what it read past the bounds of what a parser keeps', () => {
    const { compile } = createParser();
    const messages = Array.from({ length: 2048 }, (_, index) => compile(`${index}{{a}}`));

    for (const message of messages) message.resolve({ payload: { a: 'A' } });

    expect(messages.filter((message) => calls(READS, () => message.resolve({ payload: { a: 'A' } })) > 0)).toEqual([]);
  });

  it('leaves what the parser keeps as it was', () => {
    const { compile, resolve } = createParser();
    const reads = () => calls(READS, () => resolve('Hello, {{a}}!', { payload: { a: 'A' } })) > 0;
    const greeting = compile('Hello, {{a}}!');

    greeting.resolve({ payload: { a: 'A' } });
    greeting.resolve({ payload: { a: 'A' } });

    expect([reads(), reads(), reads()]).toEqual([true, true, false]);
  });
});
