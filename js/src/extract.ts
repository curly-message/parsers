import type { Modifier, Parser } from './types';
import { getModifierInput, ownModifiers, ownValue, parsePlaceholder, scanner } from './utils';
import type { Segment } from './utils';

export type { Modifier, Parser };

/**
 * What each built-in modifier narrows the value it is handed to. A modifier
 * that reads the value as text narrows it not at all and names nothing here,
 * because every value arrives as text (SPEC.md section 4) and so satisfies it.
 * The table is keyed by the registry, so a modifier this format gains is one
 * that stops compiling until it says what it reads.
 */
const NARROWED: Record<Modifier.DefaultKeys, readonly Parser.ParamKind[]> = {
  eq: [],
  ne: [],
  lt: ['number'],
  gt: ['number'],
  // The equality leg selects on text over every option before the numeric leg
  // is reached, so a value no numeric order accepts still selects one
  // (SPEC.md section 11.1).
  lte: ['number', 'string'],
  gte: ['number', 'string'],
  number: ['number'],
  currency: ['number'],
  // A signed millisecond delta relative to now, not a point in time.
  ago: ['number'],
  // Milliseconds since the epoch, and failing that text the host reads as a
  // date — which is what keeps a value authored as a date instance formattable.
  date: ['date', 'string'],
  plural: ['number'],
  // A whole number: a fraction is a value an ordinal cannot take, though the
  // kinds have no word for the difference.
  ordinal: ['number'],
};

// What the message has said about a key so far. A key several placeholders
// name says what all of them say together, each in the order it was first
// said: a placeholder adds what it says to the draft rather than copying
// everything said before it, which would cost the square of how often the
// message names the key.
type Draft = { kinds: Set<Parser.ParamKind>, values: Set<string>, optional: boolean };

const kind = (kinds: readonly Parser.ParamKind[]): Parser.ParamKind | readonly Parser.ParamKind[] => {
  if (!kinds.length) return 'unknown';

  return kinds.length === 1 ? kinds[0] : kinds;
};

export const createExtractor: Parser.ExtractorFactory = (options) => {
  // A name a host registered its own modifier under no longer answers with
  // this format's, so what a message naming it says about its value is no
  // longer something the table above knows. The registry is read the way
  // resolution reads it: an entry that is not a modifier registers none.
  const registered = ownModifiers(ownValue(options, 'customModifiers'));

  const narrowed = (modifier: string): readonly Parser.ParamKind[] => (registered[modifier] ? undefined : ownValue(NARROWED, modifier)) ?? [];

  // Only `eq` and the plural selections select on the value itself, so only
  // their keys are values the parameter takes: `ne`'s keys are what the value
  // must differ from, and an inequality's are thresholds it is ordered
  // against. A placeholder naming no modifier selects with `eq` too (SPEC.md
  // section 9.5). A plural selection's category keys are forms of the language
  // rather than values, so only its numbers are named — and of `ordinal`'s,
  // only the integers it can take (SPEC.md section 11.5).
  const exactly = (declared: readonly Segment[], takes: (input: number) => boolean) => declared.map(({ key }) => key).filter((key) => {
    const input = getModifierInput(key);

    return input !== undefined && takes(input);
  });

  const named = (modifier: string, declared: readonly Segment[]) => {
    const name = modifier || 'eq';

    if (registered[name]) return [];

    if (name === 'eq') return declared.map(({ key }) => key);

    if (name === 'plural') return exactly(declared, () => true);

    return name === 'ordinal' ? exactly(declared, Number.isInteger) : [];
  };

  return (message) => {
    const drafts = new Map<string, Draft>();

    if (typeof message !== 'string') return [];

    const scan = scanner(message);

    // An option value is the message's own text, so a placeholder written in
    // one names a parameter like any other (SPEC.md section 12). The outer
    // placeholder is written first and is drafted first, which is the order
    // the parameters come back in. The spans still to read wait on a stack
    // rather than in a call per level: nesting is not otherwise limited
    // (section 6, note 10), and a message nested deep enough must not take the
    // host's call stack down with it.
    const spans: [number, number][] = [[0, message.length]];

    for (let span = spans.pop(); span; span = spans.pop()) {
      const [from, to] = span;
      const match = scan.next(from, to);

      if (!match) continue;

      const [open, close] = match;
      const { key, modifier, options, inlineDefault } = parsePlaceholder(message, open, close, scan.end);

      // A placeholder naming no key reads no payload entry, so it names no
      // parameter itself — what it writes in its options it still names.
      if (key !== undefined) {
        const draft = drafts.get(key) ?? { kinds: new Set(), values: new Set(), optional: false };

        for (const each of narrowed(modifier)) draft.kinds.add(each);

        for (const each of named(modifier, options)) draft.values.add(each);

        draft.optional ||= inlineDefault !== undefined;
        drafts.set(key, draft);
      }

      // The spans the placeholder holds message text in, read in the order
      // the message writes them and before the rest of the span: an option
      // and the inline default are one sequence of segments, and a parameter a
      // nested placeholder names comes back where the message names it.
      const held = [...options.map(({ value }) => value), ...(inlineDefault ? [inlineDefault] : [])].sort(([a], [b]) => a - b);

      spans.push([close, to]);

      for (let index = held.length - 1; index >= 0; index -= 1) spans.push(held[index]);
    }

    return [...drafts].map(([name, draft]) => ({
      name,
      kind: kind([...draft.kinds]),
      ...(draft.values.size ? { values: [...draft.values] } : {}),
      optional: draft.optional,
    }));
  };
};
