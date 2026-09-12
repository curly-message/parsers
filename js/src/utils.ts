import type { Modifier, Report } from './types';

/**
 * The format's `line-term` production (SPEC.md section 6, note 1): the four
 * code points a placeholder holds in no position. Every rule that reads the
 * set reads it from here, so an amendment moves one list rather than four.
 * Written as escapes: a literal terminator is invisible to review and to a
 * diff.
 */
export const LINE_TERM = ['\u000a', '\u000d', '\u2028', '\u2029'];

// The ladder `ago` climbs, each step a multiple of the one below it. A `format`
// naming a unit `Intl` knows but this ladder does not climb is one the
// modifier refuses, so the ladder is also what `Modifier.AgoUnit` accepts as a
// format, read straight off this list. It lives here rather than
// beside `ago` because `Modifier.DefaultKeys` is the modifier module's
// exports: a table exported there would be typed as a modifier name.
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

const TERMINATOR_CLASS = `[${LINE_TERM.map(unicodeEscape).join('')}]`;

const TERMINATOR = new RegExp(TERMINATOR_CLASS);

/**
 * Every line terminator in a text at once, for a diagnostic that escapes them
 * all. It reads the same list the scanner does, so amending `line-term`
 * reaches both.
 */
export const EVERY_TERMINATOR = new RegExp(TERMINATOR_CLASS, 'g');

// A backslash consumes the character after it, so a brace an escape claimed
// is text rather than half of a delimiter, and a placeholder holds no line
// terminator in any position. Both delimiters are two characters, so each scan
// reads one character ahead, and `charAt` stops at the end of the message where
// an index would answer for its prototype.
const placeholderEnd = (value: string, open: number) => {
  for (let index = open + 2; index < value.length; index += 1) {
    const character = value[index];

    if (character === '\\') {
      if (TERMINATOR.test(value.charAt(index + 1))) return undefined;

      index += 1;
      continue;
    }

    if (TERMINATOR.test(character)) return undefined;

    if (character === '{' && value.charAt(index + 1) === '{') return undefined;

    if (character === '}' && value.charAt(index + 1) === '}') return index + 2;
  }

  return undefined;
};

/**
 * Where the next placeholder opens and where it ends, or nothing where the
 * text holds none from `from` on.
 *
 * Both scans skip an escape sequence whole, so the parity of a run of
 * backslashes is never counted backwards and the cost stays linear in the
 * length of the message. An attempt that fails leaves the braces it rejected
 * to a later pair.
 */
export const nextPlaceholder = (value: string, from: number): [number, number] | undefined => {
  for (let index = from; index < value.length; index += 1) {
    if (value[index] === '\\') {
      index += 1;
      continue;
    }

    if (value[index] !== '{' || value.charAt(index + 1) !== '{') continue;

    const end = placeholderEnd(value, index);

    if (end !== undefined) return [index, end];
  }

  return undefined;
};

// The syntax reserves a colon, a semicolon, either brace, a backslash and
// whitespace, which `isBlank` answers for. A backslash writes any of them as
// text; before anything else it is text itself, so a Windows path and a
// regular expression survive as typed.
const RESERVED = /[:;{}\\]/;

/** Text as the format reads it, with every escape sequence it claims resolved. */
export const unesc = (value: any) => typeof value === 'string' ? value.replace(/\\([\s\S])/g, (sequence, character) => RESERVED.test(character) || isBlank(character) ? character : sequence) : value;

// Whitespace an escape sequence claims is text, not padding around it.
const trim = (value: string) => {
  let start = -1;
  let end = 0;

  for (let index = 0; index < value.length; index += 1) {
    const escaped = value[index] === '\\' && index + 1 < value.length;

    if (!escaped && isBlank(value[index])) continue;

    if (start < 0) start = index;

    index += escaped ? 1 : 0;
    end = index + 1;
  }

  return start < 0 ? '' : value.slice(start, end);
};

// Separates on every occurrence of `separator` no escape sequence claims.
const split = (value: string, separator: string) => {
  const parts: string[] = [];
  let from = 0;

  for (let index = 0; index < value.length; index += 1) {
    if (value[index] === '\\') index += 1;
    else if (value[index] === separator) {
      parts.push(value.slice(from, index));
      from = index + 1;
    }
  }

  return [...parts, value.slice(from)];
};

/**
 * What a placeholder declares: the payload key it names, the modifier it
 * names, the options it carries and the default it states inline. Every name
 * arrives unescaped, because a name answers to itself rather than to the
 * spelling a message needed to write it.
 *
 * Reading a placeholder is the same work whether it is being resolved or only
 * described, so resolution and extraction read it here.
 */
export const parsePlaceholder = (placeholder: string): { key?: string, modifier: string, options: Modifier.ModifierOption[], inlineDefault?: string } => {
  const [declaration, ...declaredOptions] = split(placeholder.slice(2, -2), ';');
  const [declaredKey, ...declaredModifier] = split(declaration, ':');

  const declaredName = trim(declaredKey);
  const options: Modifier.ModifierOption[] = [];
  let inlineDefault: string | undefined;

  declaredOptions.forEach((option) => {
    const [declaredOptionKey, ...declaredValue] = split(option, ':');
    const optionKey = unesc(trim(declaredOptionKey));
    // The first colon is the separator and every later one is value. An
    // option that names no value at all stands for itself; one that ends
    // at its colon declares the empty string.
    const optionValue = declaredValue.length ? trim(declaredValue.join(':')) : trim(declaredOptionKey);

    if (!optionKey) return;

    // `default` is reserved in lowercase, so both gates read the same
    // spelling and a segment is either the inline default or an option.
    if (inlineDefault === undefined && optionKey === 'default') inlineDefault = optionValue;

    if (optionKey !== 'default') options.push({ key: optionKey, value: optionValue });
  });

  return { key: declaredName ? unesc(declaredName) : undefined, modifier: unesc(trim(declaredModifier.join(':'))), options, inlineDefault };
};

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

/**
 * The names a target carries of its own. Enumerating them reads the target as
 * surely as reading one does, so a target that refuses the question answers
 * nothing and says so the same way.
 */
export const ownKeys = (target: any, onRaise?: () => void) => {
  if (!target) return [];

  try {
    return Object.keys(target);
  } catch {
    onRaise?.();

    return [];
  }
};

export const mergeLayer = (base: any, override: any, onRaise?: () => void) => {
  // A null prototype takes `__proto__` as an own key instead of routing the
  // name through the prototype setter, and the merged layer keeps that
  // prototype on the way out: whatever reads it — a host formatter, a modifier
  // — then answers for the entries it was configured with and for no others.
  const output: Record<string, any> = Object.create(null);

  ownKeys(base, onRaise).forEach((name) => { output[name] = ownValue(base, name, onRaise); });

  ownKeys(override, onRaise).forEach((name) => {
    const to = ownValue(override, name, onRaise);

    // A name the override sets to `undefined` names nothing, like one it omits.
    if (to === undefined) return;

    output[name] = to;
  });

  return output;
};

/**
 * The modifiers a registry holds. An entry that is not one registers nothing:
 * it takes no name of its own and shadows no modifier the layer beneath it
 * holds under that name. A name no layer holds a modifier under is a name
 * nobody registered, which is what a message writing it reads.
 */
export const ownModifiers = (registry: any, onRaise?: () => void) => {
  const output: Record<string, any> = Object.create(null);

  ownKeys(registry, onRaise).forEach((name) => {
    const entry = ownValue(registry, name, onRaise);

    if (typeof entry === 'function') output[name] = entry;
  });

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
