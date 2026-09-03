import type { Report } from './types';

/**
 * The format's `line-term` production (SPEC.md section 6, note 1): the four
 * code points a placeholder holds in no position. Every rule that reads the
 * set reads it from here, so an amendment moves one list rather than four.
 * Written as escapes: a literal terminator is invisible to review and to a
 * diff.
 */
export const LINE_TERM = ['\u000a', '\u000d', '\u2028', '\u2029'];

// The ladder `ago` climbs, each step a multiple of the one below it. A unit
// `Intl` knows but this ladder does not climb can never be selected — the climb
// simply runs out at `year` — so the ladder is also what `Modifier.AgoProps`
// accepts as a format, read straight off this list. It lives here rather than
// beside `ago` because the modifier module's exports are the modifier
// registry: a table exported there would answer to its own name in a message.
export const AGO_LADDER = [
  { key: 'second', multiplier: 1000 },
  { key: 'minute', multiplier: 60 },
  { key: 'hour', multiplier: 60 },
  { key: 'day', multiplier: 24 },
  { key: 'week', multiplier: 7 },
  { key: 'month', multiplier: 13 / 3 },
  { key: 'year', multiplier: 12 },
] as const satisfies readonly { key: Intl.RelativeTimeFormatUnit, multiplier: number }[];

/**
 * A code point as the four-digit escape a JSON string carries, which is how
 * both a regular expression source and a diagnostic excerpt name one.
 */
export const unicodeEscape = (character: string) => `\\u${character.charCodeAt(0).toString(16).padStart(4, '0')}`;

/**
 * Whether text carries nothing outside the whitespace class SPEC.md section 6
 * enumerates and forbids substituting. The host's own notion is no substitute
 * anyway: it is defined over a live Unicode general category and has changed
 * membership before.
 */
export const isBlank = (value: string) => !/[^\t\n\v\f\r\u0020\u00a0\u1680\u2000-\u200a\u2028\u2029\u202f\u205f\u3000\ufeff]/.test(value);

/**
 * A target's own enumerable entry under a key, or nothing. Section 4's value
 * conversion reads by enumerability already, so every read the parser makes
 * reads by it: an entry `Object.defineProperty` left hidden is not one the
 * format carries, wherever it sits.
 *
 * A read that raises answers nothing too — resolution must not throw — but a
 * caller that can tell the difference between an entry nobody passed and one
 * that refused to be read says so through `onRaise`.
 */
export const ownValue = (target: any, key?: PropertyKey, onRaise?: () => void) => {
  try {
    return key !== undefined && !!target && Object.prototype.propertyIsEnumerable.call(target, key) ? target[key] : undefined;
  } catch {
    onRaise?.();

    return undefined;
  }
};

export const ownKeys = (target: any) => {
  try {
    return Object.keys(target);
  } catch {
    return [];
  }
};

export const mergeLayer = (base: any, override: any, merge?: (from: any, to: any) => any) => {
  // A null prototype takes `__proto__` as an own key instead of routing the
  // name through the prototype setter, and the merged layer keeps that
  // prototype on the way out: whatever reads it — a host formatter, a modifier
  // — then answers for the entries it was configured with and for no others.
  const output: Record<string, any> = Object.create(null);

  ownKeys(base).forEach((name) => { output[name] = ownValue(base, name); });

  ownKeys(override).forEach((name) => {
    const to = ownValue(override, name);

    // A name the override sets to `undefined` names nothing, like one it omits.
    if (to === undefined) return;

    output[name] = merge ? merge(ownValue(base, name), to) : to;
  });

  return output;
};

/**
 * The modifiers a registry holds. An entry that is not one registers nothing:
 * it takes no name of its own and shadows no modifier the layer beneath it
 * holds under that name. A name no layer holds a modifier under is a name
 * nobody registered, which is what a message writing it reads.
 */
export const ownModifiers = (registry: any) => {
  const output: Record<string, any> = Object.create(null);

  ownKeys(registry).forEach((name) => {
    const entry = ownValue(registry, name);

    if (typeof entry === 'function') output[name] = entry;
  });

  return { ...output };
};

// A configuration layer is read the way the payload is: own properties only,
// one level at a time. Nobody writes configuration onto a prototype, so
// anything a prototype offers here was put there by someone else.
export const ownLayer = (target: any, key: PropertyKey) => {
  const layer = ownValue(target, key);
  const output: Record<string, any> = Object.create(null);

  ownKeys(layer).forEach((name) => { output[name] = ownValue(layer, name); });

  return output;
};

/**
 * Why a modifier could not answer, in the vocabulary a report is written in. A
 * built-in modifier says so by raising, the way a host-defined one already
 * does, and the parser reads the code off the raise. It never leaves the
 * parser.
 */
export class ModifierFailure extends Error {
  constructor(readonly code: Report['code']) {
    super(code);
  }
}

/**
 * The code a raise carries, where the raise is one of the parser's own. A
 * modifier may raise anything at all — an object that raises again at the mere
 * question of what it is included — so nothing is one of the answers.
 */
export const failureCode = (raised: any): Report['code'] | undefined => {
  try {
    return raised instanceof ModifierFailure ? raised.code : undefined;
  } catch {
    return undefined;
  }
};

/**
 * The number a formatting modifier will format, by the host's own conversion.
 * A value that does not convert is one the modifier cannot format — the
 * placeholder resolves to the fallback chain instead.
 */
export const getModifierInput = (value: any) => {
  // `+''` is `0`, so blank text would otherwise format as a number nobody wrote.
  if (typeof value === 'string' && isBlank(value)) return undefined;

  const input = +value;

  return Number.isFinite(input) ? input : undefined;
};

/**
 * The timestamp the `date` modifier will format. Numeric text is a timestamp
 * already; anything else is left to the host's own `Date` parsing, so an ISO
 * string and the form `String(new Date())` writes both read as dates.
 */
export const getDateInput = (value: any) => {
  const timestamp = getModifierInput(value);

  if (timestamp !== undefined) return timestamp;

  const parsed = Date.parse(value);

  return Number.isNaN(parsed) ? undefined : parsed;
};
