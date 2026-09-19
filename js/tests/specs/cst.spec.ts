import { describe, expect, it } from 'vitest';
import { decode, fixtures } from '@curly-message/conformance';
import type { Case, TreeCase } from '@curly-message/conformance';
import { createExtractor, cst } from '../../src';
import type { Cst } from '../../src';
import { MESSAGES } from '../data';
import { LINE_TERM, parsePlaceholder, unesc } from '../../src/utils';

const leaves = (node: Cst.Node): Cst.Node[] => ('nodes' in node && node.nodes.length ? node.nodes.flatMap(leaves) : [node]);

const kinds = (message: string) => cst(message).nodes.map(({ type }) => type);

const of = (message: string, type: Cst.Name['type']) =>
  cst(message)
    .nodes.filter((node): node is Cst.Placeholder => node.type === 'placeholder')
    .flatMap(({ nodes }) => nodes)
    .filter((node): node is Cst.Name => node.type === type);

// A generated case names a construction rather than a message, so only the
// cases that state one are read, each through the set's own decoding. A file
// of either kind is read: a tree case states a message the same way, and the
// two kinds are what one corpus is assembled from.
const STATED: string[] = fixtures()
  .flatMap(({ file }): readonly (Case | TreeCase)[] => file.cases)
  .map((stated) => ('message' in stated ? decode(stated.message) : undefined))
  .filter((message): message is string => typeof message === 'string');

const CATALOGUED: string[] = Object.values(MESSAGES).flatMap((locale: any) => Object.values<any>(locale).flatMap((catalogue: any) => Object.values<string>(catalogue)));

// Every message the conformance set states, every message the test catalogues
// hold, and the spellings a set of resolutions has no reason to carry.
const CORPUS: string[] = [
  ...STATED,
  ...CATALOGUED,
  '',
  '\\',
  '{{',
  '}}',
  '{{}}',
  '{{;}}',
  '{{:}}',
  '{{ }}',
  '{{v\\}}}',
  '\\{{v}}',
  'a {{v:number; a:1; default:\\;;}} b',
  `{{v${LINE_TERM[0]}}}`,
  '{{a:b:c; d:e:f;}}',
  '\\\u{1f600}x',
  '{{v; a:\\\u{1f600};}}',
];

// A boundary between the halves of a surrogate pair cuts a character in two.
// Contiguity does not see it — both halves still lie in a leaf — so it is
// asserted on its own.
const cuts = (message: string, at: number) => at > 0 && at < message.length && message.charCodeAt(at - 1) >= 0xd800 && message.charCodeAt(at - 1) <= 0xdbff && message.charCodeAt(at) >= 0xdc00 && message.charCodeAt(at) <= 0xdfff;

// What a placeholder declares, read off the tree the way resolution reads it
// off the text. An option that names nothing declares nothing, and one named
// `default` is the inline default rather than an option (section 9.4).
const stated = (placeholder: Cst.Placeholder) => {
  const named = placeholder.nodes.filter((node): node is Cst.Name => 'name' in node);
  const options: { key: string, value: string }[] = [];
  let inlineDefault: string | undefined;

  named.forEach((node, index) => {
    if (node.type !== 'option-key' || !node.name) return;

    const next = named[index + 1];
    // A segment that states no value stands for its own key.
    const value = next?.type === 'option-value' ? next.name : node.name;

    if (inlineDefault === undefined && node.name === 'default') inlineDefault = value;
    if (node.name !== 'default') options.push({ key: node.name, value });
  });

  return {
    key: named.find(({ type }) => type === 'key')?.name || undefined,
    modifier: named.find(({ type }) => type === 'modifier')?.name ?? '',
    options,
    inlineDefault,
  };
};

describe('the tree a message is described by', () => {
  it('reads the whole corpus the invariants below are asserted over', () => {
    // The two property tests assert nothing over a corpus that came back
    // empty, and a conformance case that stops carrying its message, or a
    // catalogue that gains a level, would empty one silently.
    expect(STATED.length).toBeGreaterThan(600);
    expect(CATALOGUED.length).toBeGreaterThan(20);
  });

  it('lays every character of the message in exactly one leaf, in order', () => {
    for (const message of CORPUS) {
      const spans = leaves(cst(message));
      let at = 0;

      for (const { start, end } of spans) {
        expect([message, start]).toEqual([message, at]);
        expect(end).toBeGreaterThanOrEqual(start);
        expect([message, start, cuts(message, start)]).toEqual([message, start, false]);
        at = end;
      }

      expect([message, at]).toEqual([message, message.length]);
      expect(spans.map(({ start, end }) => message.slice(start, end)).join('')).toBe(message);
    }
  });

  it('describes a message longer than a call can be spread over', () => {
    const message = '\\a'.repeat(150000);

    expect(cst(message).nodes).toHaveLength(150000);
  });

  it('describes a message that is not text as one with nothing in it', () => {
    for (const message of [undefined, null, 42, {}, ['{{v}}']]) expect(cst(message)).toEqual({ type: 'message', start: 0, end: 0, nodes: [] });
  });

  it('holds text and placeholders, and nothing between them', () => {
    expect(kinds('Hello, {{name;}}!')).toEqual(['text', 'placeholder', 'text']);
    expect(kinds('{{a}}{{b}}')).toEqual(['placeholder', 'placeholder']);
    expect(kinds('Hello.')).toEqual(['text']);
    expect(kinds('')).toEqual([]);
  });

  it('describes a placeholder by its parts, in the order the message writes them', () => {
    const [placeholder] = cst('{{count:number; a:1;}}').nodes;

    expect(placeholder.type).toBe('placeholder');
    expect((placeholder as Cst.Placeholder).nodes.map(({ type }) => type)).toEqual([
      'open',
      'key',
      'separator',
      'modifier',
      'separator',
      'space',
      'option-key',
      'separator',
      'option-value',
      'separator',
      'option-key',
      'close',
    ]);
  });

  it('names a key and a modifier by what they answer to, not by how they are spelled', () => {
    expect(of('{{my\\;key:number;}}', 'key').map(({ name }) => name)).toEqual(['my;key']);
    expect(of('{{ v : number ;}}', 'key').map(({ name, start, end }) => [name, start, end])).toEqual([['v', 3, 4]]);
    expect(of('{{ v : number ;}}', 'modifier').map(({ name }) => name)).toEqual(['number']);
  });

  it('reads the selector to the first colon and the modifier through every later one', () => {
    expect(of('{{a:b:c;}}', 'key').map(({ name }) => name)).toEqual(['a']);
    expect(of('{{a:b:c;}}', 'modifier').map(({ name }) => name)).toEqual(['b:c']);
  });

  it('reads an option to its first colon and the value through every later one', () => {
    expect(of('{{v; a:b:c;}}', 'option-key').map(({ name }) => name)).toEqual(['a', '']);
    expect(of('{{v; a:b:c;}}', 'option-value').map(({ name }) => name)).toEqual(['b:c']);
  });

  it('describes a segment that states no value by its key alone', () => {
    expect(of('{{v; shipped;}}', 'option-key').map(({ name }) => name)).toEqual(['shipped', '']);
    expect(of('{{v; shipped;}}', 'option-value')).toEqual([]);
    expect(of('{{v; shipped:;}}', 'option-value').map(({ name }) => name)).toEqual(['']);
    // The `;` an idiomatic placeholder ends with opens a segment of its own,
    // and that segment is empty: section 6 derives `{ ";" , segment }`.
    expect(of('{{v; shipped:;}}', 'option-key').map(({ name }) => name)).toEqual(['shipped', '']);
  });

  it('gives an empty name a position of its own', () => {
    expect(of('{{}}', 'key').map(({ name, start, end }) => [name, start, end])).toEqual([['', 2, 2]]);
    expect(of('{{  }}', 'key').map(({ start, end }) => [start, end])).toEqual([[2, 2]]);
  });

  it('says of an escape sequence whether it cancels a meaning or denotes itself', () => {
    const escapes = (message: string) => leaves(cst(message)).filter((node): node is Cst.Escape => node.type === 'escape');

    expect(escapes('\\; \\{ \\\\ \\ ').map(({ cancels }) => cancels)).toEqual([true, true, true, true]);
    expect(escapes('\\d\\a').map(({ cancels }) => cancels)).toEqual([false, false]);
    // Every member of the whitespace class escapes, the line terminators among
    // them, and those are the ones a host notion of whitespace leaves out.
    expect(escapes(LINE_TERM.map((terminator) => `\\${terminator}`).join('x')).map(({ cancels }) => cancels)).toEqual(LINE_TERM.map(() => true));
    // A backslash with nothing left to consume denotes itself, and is text.
    expect(kinds('a\\')).toEqual(['text']);
  });

  it('lets a sequence claim a whole character, not one half of a surrogate pair', () => {
    const message = '\\\u{1f600}x';
    const [escape, text] = cst(message).nodes;

    expect([escape.type, message.slice(escape.start, escape.end)]).toEqual(['escape', '\\\u{1f600}']);
    expect([text.type, message.slice(text.start, text.end)]).toEqual(['text', 'x']);
  });

  it('describes a construct enclosing another as the text section 12 resolves it as', () => {
    const message = '{{count:gt; 0:{{count:number;}}; default:no;}}';
    const [text, placeholder, rest] = cst(message).nodes;

    expect([text.type, placeholder.type, rest.type]).toEqual(['text', 'placeholder', 'text']);
    expect(message.slice(placeholder.start, placeholder.end)).toBe('{{count:number;}}');
  });

  it('derives no placeholder where a backslash consumed a brace (appendix A.15 and A.16)', () => {
    const derives = (message: string) => kinds(message).includes('placeholder');

    expect(derives('\\{{v}}')).toBe(false);
    expect(derives('{{v\\}}')).toBe(false);
    // The `}` the backslash consumed is content, so the pair that closes is
    // the next one and the key holds a brace.
    expect(derives('{{v\\}}}')).toBe(true);
    expect(of('{{v\\}}}', 'key').map(({ name }) => name)).toEqual(['v}']);
  });

  it('carries no placeholder across a line terminator (section 6, note 1)', () => {
    for (const terminator of LINE_TERM) expect(kinds(`{{v${terminator}}}`)).toEqual(['text']);
  });

  it('declares what parsing a placeholder declares, over every message the set states', () => {
    for (const message of CORPUS) {
      for (const node of cst(message).nodes) {
        if (node.type !== 'placeholder') continue;

        const source = message.slice(node.start, node.end);
        const read = parsePlaceholder(source);

        // An option value is the source spelling, unescaped once with the rest
        // of the output rather than matched by name (section 7), so that is
        // where the two readings meet.
        expect([source, stated(node)]).toEqual([source, {
          key: read.key,
          modifier: read.modifier,
          options: read.options.map(({ key, value }) => ({ key, value: unesc(value) })),
          inlineDefault: read.inlineDefault === undefined ? undefined : unesc(read.inlineDefault),
        }]);
      }
    }
  });

  it('names the keys the extractor names, over every message the set states', () => {
    const extract = createExtractor();

    for (const message of CORPUS) {
      const named = [
        ...new Set(
          cst(message)
            .nodes.filter((node): node is Cst.Placeholder => node.type === 'placeholder')
            .flatMap(({ nodes }) => nodes.filter((node): node is Cst.Name => node.type === 'key'))
            .map(({ name }) => name)
            .filter((name) => name !== ''),
        ),
      ];

      expect([message, named]).toEqual([message, extract(message).map(({ name }) => name)]);
    }
  });
});
