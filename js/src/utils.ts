import type { Report } from './types';

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
 * Where the escape sequence opening at `index` ends. A backslash consumes the
 * character after it (section 7), and a character outside the basic plane is
 * written as two code units, so the pair is claimed whole rather than cut in
 * half. Every scan that steps over a sequence reads its extent here, so the
 * rule has one implementation and not one per scanner.
 */
export const escapeEnd = (value: string, index: number, to: number) => (index + 2 < to && value.codePointAt(index + 1)! > 0xffff ? index + 3 : index + 2);

/**
 * Every line terminator in a text at once, for a diagnostic that escapes them
 * all. It reads the same list the scanner does, so amending `line-term`
 * reaches both.
 */
export const EVERY_TERMINATOR = new RegExp(TERMINATOR_CLASS, 'g');

// Which part of a placeholder the scan is inside. Only a value admits a
// nested placeholder (SPEC.md section 6, note 10), so the scan carries the
// part to know whether a `{{` it meets opens one or refuses the construct.
type Part = 'key' | 'modifier' | 'option-key' | 'option-value';

/**
 * Reads one message for the placeholders it derives. It is scoped to that
 * message because it records what it has decided, and a verdict belongs to a
 * message and a position rather than to the parser.
 */
export type Scanner = {
  /** Where a complete placeholder opening at `open` ends, or nothing. */
  end: (open: number) => number | undefined,
  /**
   * Where the next placeholder opening at or after `from` opens and ends, or
   * nothing. The search stops at `to`, so a span of the message is read for
   * what it holds rather than for what follows it.
   */
  next: (from: number, to: number) => [number, number] | undefined,
};

/**
 * A scanner for one message.
 *
 * A backslash consumes the character after it, so a brace an escape claimed is
 * text rather than half of a delimiter, and a placeholder holds no line
 * terminator in any position. Both delimiters are two characters, so the scan
 * reads one character ahead, and `charAt` stops at the end of the message
 * where an index would answer for its prototype.
 *
 * A verdict is final (section 6, note 11): whether a placeholder derives at a
 * position is a function of the message and that position alone, so an answer
 * is recorded and never recomputed. That record is what keeps the scan linear
 * in the length of the message — note 7 has the scan resume one code point
 * past a brace that opened nothing, so the attempts overlap, and without the
 * record a message that nests deeply costs time growing faster than any
 * polynomial in its length.
 */
export const scanner = (value: string): Scanner => {
  // A position answered `-1` derives no placeholder. Absent means unasked.
  const verdict = new Map<number, number>();

  const end = (open: number) => {
    const known = verdict.get(open);

    if (known !== undefined) return known < 0 ? undefined : known;

    // An explicit stack rather than recursion: nesting is not otherwise
    // limited (section 6, note 10), and a message twenty thousand levels deep
    // must not take the host's call stack down with it.
    const stack: { open: number, index: number, part: Part }[] = [{ open, index: open + 2, part: 'key' }];
    // Every construct still open when the scan dies is one whose closing pair
    // never arrived, so each of them derives nothing.
    const refuse = () => {
      stack.forEach((frame) => verdict.set(frame.open, -1));

      return undefined;
    };

    while (stack.length) {
      const frame = stack[stack.length - 1];

      if (frame.index >= value.length) return refuse();

      const character = value[frame.index];

      if (character === '\\') {
        if (TERMINATOR.test(value.charAt(frame.index + 1))) return refuse();

        frame.index = escapeEnd(value, frame.index, value.length);
        continue;
      }

      if (TERMINATOR.test(character)) return refuse();

      if (character === ';') {
        frame.part = 'option-key';
        frame.index += 1;
        continue;
      }

      if (character === ':') {
        // The first colon of a part is its separator and a later one is
        // content (section 6, notes 3 and 4), so a part only ever moves on.
        if (frame.part === 'key') frame.part = 'modifier';
        else if (frame.part === 'option-key') frame.part = 'option-value';

        frame.index += 1;
        continue;
      }

      if (character === '{' && value.charAt(frame.index + 1) === '{') {
        // A `{{` in a value must open a complete placeholder or the construct
        // around it does not derive at all; anywhere else it opens none, so
        // the construct around it does not derive either (note 10).
        if (frame.part !== 'option-value') return refuse();

        const nested = verdict.get(frame.index);

        if (nested !== undefined) {
          if (nested < 0) return refuse();

          frame.index = nested;
          continue;
        }

        stack.push({ open: frame.index, index: frame.index + 2, part: 'key' });
        continue;
      }

      if (character === '}' && value.charAt(frame.index + 1) === '}') {
        const close = frame.index + 2;

        verdict.set(frame.open, close);
        stack.pop();

        if (!stack.length) return close;

        stack[stack.length - 1].index = close;
        continue;
      }

      frame.index += 1;
    }

    return undefined;
  };

  const next = (from: number, to: number): [number, number] | undefined => {
    for (let index = from; index < to; index += 1) {
      if (value[index] === '\\') {
        index = escapeEnd(value, index, to) - 1;
        continue;
      }

      // Both braces belong to the span: a placeholder that derives inside one
      // closes inside it, so a lone brace at the end opens nothing.
      if (value[index] !== '{' || index + 1 >= to || value[index + 1] !== '{') continue;

      const close = end(index);

      // The braces a refused construct opened are text, and the scan resumes
      // at the very next code point rather than past the pair (note 7).
      if (close !== undefined) return [index, close];
    }

    return undefined;
  };

  return { end, next };
};

// The syntax reserves a colon, a semicolon, either brace, a backslash and
// whitespace, which `isBlank` answers for. A backslash writes any of them as
// text; before anything else it is text itself, so a Windows path and a
// regular expression survive as typed.
const RESERVED = /[:;{}\\]/;

/**
 * Whether a backslash in front of this character cancels a structural meaning
 * rather than denoting itself (SPEC.md section 7). Both readings are escape
 * sequences; they differ in what removing one leaves behind.
 */
export const cancels = (character: string) => RESERVED.test(character) || isBlank(character);

/** Text as the format reads it, with every escape sequence it claims resolved. */
export const unesc = (value: any) => typeof value === 'string' ? value.replace(/\\([\s\S])/gu, (sequence, character) => cancels(character) ? character : sequence) : value;

/**
 * The span between `from` and `to` with its blank padding dropped, empty at
 * `from` where the span is padding throughout. Whitespace an escape sequence
 * claims is text, not padding around it.
 */
export const trimmed = (value: string, from: number, to: number): [number, number] => {
  let start = -1;
  let end = from;

  for (let index = from; index < to; index += 1) {
    const escaped = value[index] === '\\' && index + 1 < to;

    if (!escaped && isBlank(value[index])) continue;

    if (start < 0) start = index;

    index = escaped ? escapeEnd(value, index, to) - 1 : index;
    end = index + 1;
  }

  return start < 0 ? [from, from] : [start, end];
};

/**
 * The spans between `from` and `to` that `separator` divides, passing over
 * every occurrence an escape sequence claims and every one a nested
 * placeholder writes (SPEC.md section 6, note 5). There is always one span: a
 * range the separator does not occur in is itself.
 *
 * `end` is the scanner's, and a caller that has none passes none: a key, a
 * modifier name and an option key hold no placeholder (note 10), so a range
 * inside one is divided by escapes alone.
 */
export const separated = (value: string, from: number, to: number, separator: string, end?: Scanner['end']): [number, number][] => {
  const parts: [number, number][] = [];
  let start = from;

  for (let index = from; index < to; index += 1) {
    if (value[index] === '\\') { index = escapeEnd(value, index, to) - 1; continue; }

    if (end && value[index] === '{' && value.charAt(index + 1) === '{') {
      const close = end(index);

      if (close !== undefined && close <= to) { index = close - 1; continue; }
    }

    if (value[index] === separator) {
      parts.push([start, index]);
      start = index + 1;
    }
  }

  return [...parts, [start, to]];
};

/** A span with its blank padding dropped, unescaped: a name as it answers. */
const named = (value: string, from: number, to: number) => {
  const [start, end] = trimmed(value, from, to);

  return { empty: start === end, name: unesc(value.slice(start, end)) as string };
};

/** An option a placeholder declares, with its value left as a span to read. */
export type Segment = {
  /** The option key, padding dropped and escapes removed. */
  key: string,
  /**
   * Where the value is written, padding dropped. A segment that states no
   * value stands for its own key (section 9.4), so the span is the key's.
   * It is a span and not text because a value is message text: it may hold a
   * placeholder, and reading it is work the modifier asks for rather than
   * work collecting it does (section 9.4).
   */
  value: [number, number],
};

/**
 * What a placeholder declares: the payload key it names, the modifier it
 * names, the options it carries and the default it states inline. Every name
 * arrives unescaped, because a name answers to itself rather than to the
 * spelling a message needed to write it. Every value arrives unread.
 *
 * Reading a placeholder is the same work whether it is being resolved or only
 * described, so resolution and extraction read it here.
 */
export const parsePlaceholder = (message: string, open: number, close: number, end: Scanner['end']): { key?: string, modifier: string, options: Segment[], inlineDefault?: [number, number] } => {
  const [declaration, ...segments] = separated(message, open + 2, close - 2, ';', end);
  // The selector's colon is the first one no escape claims, and everything
  // after it is the modifier name (section 6, note 3).
  const [declaredKey] = separated(message, declaration[0], declaration[1], ':');
  const declaredName = named(message, declaredKey[0], declaredKey[1]);
  const declaredModifier = named(message, Math.min(declaredKey[1] + 1, declaration[1]), declaration[1]);

  const options: Segment[] = [];
  let inlineDefault: [number, number] | undefined;

  segments.forEach(([from, to]) => {
    const [declaredOptionKey] = separated(message, from, to, ':', end);
    const optionKey = named(message, declaredOptionKey[0], declaredOptionKey[1]).name;
    // The first colon is the separator and every later one is value. An
    // option that names no value at all stands for itself; one that ends
    // at its colon declares the empty string.
    const declaredValue: [number, number] = declaredOptionKey[1] < to ? [declaredOptionKey[1] + 1, to] : declaredOptionKey;
    const value = trimmed(message, declaredValue[0], declaredValue[1]);

    if (!optionKey) return;

    // `default` is reserved in lowercase, so both gates read the same
    // spelling and a segment is either the inline default or an option.
    if (inlineDefault === undefined && optionKey === 'default') inlineDefault = value;

    if (optionKey !== 'default') options.push({ key: optionKey, value });
  });

  return { key: declaredName.empty ? undefined : declaredName.name, modifier: declaredModifier.name, options, inlineDefault };
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
