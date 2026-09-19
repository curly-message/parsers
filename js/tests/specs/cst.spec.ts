import { describe, expect, it } from 'vitest';
import { decode, fixtures } from '@curly-message/conformance';
import type { Case, TreeCase } from '@curly-message/conformance';
import { createExtractor, cst } from '../../src';
import type { Cst } from '../../src';
import { MESSAGES } from '../data';
import { LINE_TERM, parsePlaceholder, scanner } from '../../src/utils';

const leaves = (node: Cst.Node): Cst.Node[] => ('nodes' in node && node.nodes.length ? node.nodes.flatMap(leaves) : [node]);

const kinds = (message: string) => cst(message).nodes.map(({ type }) => type);

const parts = (message: string) =>
  cst(message)
    .nodes.filter((node): node is Cst.Placeholder => node.type === 'placeholder')
    .flatMap(({ nodes }) => nodes);

const of = (message: string, type: Cst.Name['type']) => parts(message).filter((node): node is Cst.Name => node.type === type);

// An option value answers to nobody, so it carries the subtree the message
// spells and no name to compare: what it states is the text it spans.
const valued = (message: string) => parts(message)
  .filter((node): node is Cst.OptionValue => node.type === 'option-value')
  .map(({ start, end }) => message.slice(start, end));

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
// `default` is the inline default rather than an option (section 9.4). A
// value is where it is written and not what it says, because an option value
// is message text a modifier asks for rather than a name to compare.
const stated = (placeholder: Cst.Placeholder) => {
  const declared = placeholder.nodes.filter((node): node is Cst.Name | Cst.OptionValue => node.type === 'key' || node.type === 'modifier' || node.type === 'option-key' || node.type === 'option-value');
  const name = (type: Cst.Name['type']) => declared.find((node): node is Cst.Name => node.type === type)?.name;
  const options: { key: string, value: [number, number] }[] = [];
  let inlineDefault: [number, number] | undefined;

  declared.forEach((node, index) => {
    if (node.type !== 'option-key' || !node.name) return;

    const next = declared[index + 1];
    // A segment that states no value stands for its own key.
    const value: [number, number] = next?.type === 'option-value' ? [next.start, next.end] : [node.start, node.end];

    if (inlineDefault === undefined && node.name === 'default') inlineDefault = value;
    if (node.name !== 'default') options.push({ key: node.name, value });
  });

  return {
    key: name('key') || undefined,
    modifier: name('modifier') ?? '',
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
    expect(valued('{{v; a:b:c;}}')).toEqual(['b:c']);
  });

  it('describes a segment that states no value by its key alone', () => {
    expect(of('{{v; shipped;}}', 'option-key').map(({ name }) => name)).toEqual(['shipped', '']);
    expect(valued('{{v; shipped;}}')).toEqual([]);
    expect(valued('{{v; shipped:;}}')).toEqual(['']);
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

  it('describes a placeholder an option value holds inside that value (section 6, note 10)', () => {
    const message = '{{count:gt; 0:{{count:number;}}; default:no;}}';
    const [placeholder, ...rest] = cst(message).nodes;

    expect([placeholder.type, rest]).toEqual(['placeholder', []]);
    expect(message.slice(placeholder.start, placeholder.end)).toBe(message);

    const [value] = parts(message).filter((node): node is Cst.OptionValue => node.type === 'option-value');
    const [inner, ...after] = value.nodes;

    expect([message.slice(value.start, value.end), after]).toEqual(['{{count:number;}}', []]);
    expect([inner.type, message.slice(inner.start, inner.end)]).toEqual(['placeholder', '{{count:number;}}']);
  });

  it('derives a placeholder inside an option value and nowhere else (section 6, note 10)', () => {
    const spans = (message: string) => cst(message).nodes.map(({ type, start, end }) => `${type}:${message.slice(start, end)}`);

    // A value is the one position that reads into a nested construct, so a
    // `{{` in a key, in a modifier name or in an option key belongs to no
    // construct around it: that one does not derive, and the scan resumes one
    // brace along, where the inner construct derives on its own (note 7).
    expect(spans('{{a{{b}}c}}')).toEqual(['text:{{a', 'placeholder:{{b}}', 'text:c}}']);
    expect(spans('{{a:b{{c}}d}}')).toEqual(['text:{{a:b', 'placeholder:{{c}}', 'text:d}}']);
    expect(spans('{{v; a{{b}}c:d;}}')).toEqual(['text:{{v; a', 'placeholder:{{b}}', 'text:c:d;}}']);

    // In a value the `{{` must open a complete placeholder, and where it does
    // not the construct around it does not derive either -- again leaving the
    // inner spelling to the scan that resumes.
    expect(spans('{{v; a:{{b;}}')).toEqual(['text:{{v; a:', 'placeholder:{{b;}}']);

    // Where it does, the whole construct derives and the inner one is a child
    // of the value rather than a sibling of it.
    expect(spans('{{v; a:{{b}}c;}}')).toEqual(['placeholder:{{v; a:{{b}}c;}}']);
    expect(valued('{{v; a:{{b}}c;}}')).toEqual(['{{b}}c']);
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
        const read = parsePlaceholder(message, node.start, node.end, scanner(message).end);

        expect([source, stated(node)]).toEqual([source, {
          key: read.key,
          modifier: read.modifier,
          options: read.options,
          inlineDefault: read.inlineDefault,
        }]);
      }
    }
  });

  it('names the keys the extractor names, over every message the set states', () => {
    const extract = createExtractor();
    // A placeholder an option value holds is described inside that value, so
    // the keys are read through the whole tree rather than off its top row.
    const keys = (node: Cst.Node): string[] => [
      ...(node.type === 'key' && node.name !== '' ? [node.name] : []),
      ...('nodes' in node ? node.nodes.flatMap(keys) : []),
    ];

    for (const message of CORPUS) {
      expect([message, [...new Set(keys(cst(message)))]]).toEqual([message, extract(message).map(({ name }) => name)]);
    }
  });
});
