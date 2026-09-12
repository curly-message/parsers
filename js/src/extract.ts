import type { Modifier, Parser } from './types';
import { nextPlaceholder, ownModifiers, ownValue, parsePlaceholder } from './utils';

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
  const named = (modifier: string, declared: Modifier.ModifierOption[]) => (!registered.eq && (modifier === 'eq' || !modifier) ? declared.map(({ key }) => key) : []);

  return (message) => {
    const drafts = new Map<string, Draft>();

    if (typeof message !== 'string') return [];

    let from = 0;

    for (let match = nextPlaceholder(message, from); match; match = nextPlaceholder(message, from)) {
      const [open, end] = match;

      from = end;

      const { key, modifier, options, inlineDefault } = parsePlaceholder(message.slice(open, end));

      // A placeholder naming no key reads no payload entry, so it names no
      // parameter either.
      if (key === undefined) continue;

      const draft = drafts.get(key) ?? EMPTY;

      drafts.set(key, {
        kinds: [...new Set([...draft.kinds, ...narrowed(modifier)])],
        values: [...new Set([...draft.values, ...named(modifier, options)])],
        optional: draft.optional || inlineDefault !== undefined,
      });
    }

    return [...drafts].map(([name, draft]) => ({
      name,
      kind: kind(draft),
      ...(draft.values.length ? { values: draft.values } : {}),
      optional: draft.optional,
    }));
  };
};
