import type { Parser } from './types';

/**
 * The format's `line-term` production (SPEC.md section 6, note 1): the four
 * code points a placeholder holds in no position. Every rule that reads the
 * set reads it from here, so an amendment moves one list rather than four.
 * Written as escapes: a literal terminator is invisible to review and to a
 * diff.
 */
export const LINE_TERM = ['\u000a', '\u000d', '\u2028', '\u2029'];

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

export const ownValue = (target: any, key?: PropertyKey) => {
  try {
    return key !== undefined && !!target && Object.prototype.hasOwnProperty.call(target, key) ? target[key] : undefined;
  } catch {
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
  // A null prototype takes `__proto__` as an own key, and the spread that
  // finishes the object defines rather than assigns, so a supplied name stays
  // an own property instead of reaching a prototype.
  const output: Record<string, any> = Object.create(null);

  ownKeys(base).forEach((name) => { output[name] = ownValue(base, name); });

  ownKeys(override).forEach((name) => {
    const to = ownValue(override, name);

    // A name the override sets to `undefined` names nothing, like one it omits.
    if (to === undefined) return;

    output[name] = merge ? merge(ownValue(base, name), to) : to;
  });

  return { ...output };
};

export const getModifierDefaults = <T>(key: keyof T, parserOptions: Parser.Options) => {
  const { modifierDefaults } = parserOptions || {};
  const { [key]: output } = modifierDefaults || {};

  return (output || {}) as Required<T>[keyof T];
};

/**
 * The number a formatting modifier will format, by the host's own conversion.
 * A value that does not convert is one the modifier cannot format — the caller
 * resolves it to the fallback chain instead.
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
