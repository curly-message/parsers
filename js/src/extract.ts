import type { Modifier, Parser } from './types';
import { ownModifiers, ownValue, parsePlaceholder, scanner } from './utils';
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
};

// What the message has said about a key so far. A key several placeholders
// name says what all of them say together.
type Draft = { kinds: readonly Parser.ParamKind[], values: readonly string[], optional: boolean };

const EMPTY: Draft = { kinds: [], values: [], optional: false };

const kind = ({ kinds }: Draft): Parser.ParamKind | readonly Parser.ParamKind[] => {
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

  // Only `eq` selects on the value itself, so only its option keys are values
  // the parameter takes: `ne`'s keys are what the value must differ from, and
  // an inequality's are thresholds it is ordered against. A placeholder naming
  // no modifier selects with `eq` too (SPEC.md section 9.5).
  const named = (modifier: string, declared: readonly Segment[]) => (!registered.eq && (modifier === 'eq' || !modifier) ? declared.map(({ key }) => key) : []);

  return (message) => {
    const drafts = new Map<string, Draft>();

    if (typeof message !== 'string') return [];

    const scan = scanner(message);

    // An option value is the message's own text, so a placeholder written in
    // one names a parameter like any other (SPEC.md section 12). The outer
    // placeholder is written first and is drafted first, which is the order
    // the parameters come back in.
    const walk = (from: number, to: number) => {
      let at = from;

      for (let match = scan.next(at, to); match; match = scan.next(at, to)) {
        const [open, close] = match;

        at = close;

        const { key, modifier, options, inlineDefault } = parsePlaceholder(message, open, close, scan.end);

        // A placeholder naming no key reads no payload entry, so it names no
        // parameter itself — what it writes in its options it still names.
        if (key !== undefined) {
          const draft = drafts.get(key) ?? EMPTY;

          drafts.set(key, {
            kinds: [...new Set([...draft.kinds, ...narrowed(modifier)])],
            values: [...new Set([...draft.values, ...named(modifier, options)])],
            optional: draft.optional || inlineDefault !== undefined,
          });
        }

        // The spans the placeholder holds message text in, read in the order
        // the message writes them: an option and the inline default are one
        // sequence of segments, and a parameter a nested placeholder names
        // comes back where the message names it.
        const held = [...options.map(({ value }) => value), ...(inlineDefault ? [inlineDefault] : [])].sort(([a], [b]) => a - b);

        for (const [start, stop] of held) walk(start, stop);
      }
    };

    walk(0, message.length);

    return [...drafts].map(([name, draft]) => ({
      name,
      kind: kind(draft),
      ...(draft.values.length ? { values: draft.values } : {}),
      optional: draft.optional,
    }));
  };
};
