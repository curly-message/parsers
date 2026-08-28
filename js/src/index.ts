import * as defaultModifiers from './modifiers';
import type { Parser, Modifier, Interpolate, Interpolation, Locale, Report } from './types';
import { isBlank, LINE_TERM, mergeLayer, ownKeys, ownValue, unicodeEscape } from './utils';

export type { Parser, Modifier, Locale, Report };

const TERMINATOR_CLASS = `[${LINE_TERM.map(unicodeEscape).join('')}]`;

const TERMINATOR = new RegExp(TERMINATOR_CLASS);

const EVERY_TERMINATOR = new RegExp(TERMINATOR_CLASS, 'g');

// A backslash consumes the character after it, so a brace an escape claimed
// is text rather than half of a delimiter, and a placeholder holds no line
// terminator in any position.
const placeholderEnd = (value: string, open: number) => {
  for (let index = open + 2; index < value.length; index += 1) {
    const character = value[index];

    if (character === '\\') {
      if (TERMINATOR.test(value.charAt(index + 1))) return undefined;

      index += 1;
      continue;
    }

    if (TERMINATOR.test(character)) return undefined;

    if (character === '{' && value[index + 1] === '{') return undefined;

    if (character === '}' && value[index + 1] === '}') return index + 2;
  }

  return undefined;
};

// Both scans skip an escape sequence whole, so the parity of a run of
// backslashes is never counted backwards and the cost stays linear in the
// length of the message. An attempt that fails leaves the braces it rejected
// to a later pair.
const nextPlaceholder = (value: string, from: number): [number, number] | undefined => {
  for (let index = from; index < value.length; index += 1) {
    if (value[index] === '\\') {
      index += 1;
      continue;
    }

    if (value[index] !== '{' || value[index + 1] !== '{') continue;

    const end = placeholderEnd(value, index);

    if (end !== undefined) return [index, end];
  }

  return undefined;
};

const hasPlaceholders = (value: any) => typeof value === 'string' && !!nextPlaceholder(value, 0);

// The syntax reserves a colon, a semicolon, either brace, a backslash and
// whitespace, which `isBlank` answers for. A backslash writes any of them as
// text; before anything else it is text itself, so a Windows path and a
// regular expression survive as typed.
const RESERVED = /[:;{}\\]/;

const unesc = (value: any) => typeof value === 'string' ? value.replace(/\\([\s\S])/g, (sequence, character) => RESERVED.test(character) || isBlank(character) ? character : sequence) : value;

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

// A `Date`, a `RegExp` and a `Map` all say what they are through `toString`; a
// plain object says `[object Object]`, so it is the one shape JSON describes
// better.
const isPlainObject = (value: any) => {
  if (!value || typeof value !== 'object') return false;

  // A value the host will not describe is not a shape this can read.
  try {
    const prototype = Object.getPrototypeOf(value);

    return prototype === Object.prototype || prototype === null;
  } catch {
    return false;
  }
};

/**
 * The text a value resolves to. Everything the format carries is text: a plain
 * object and an array become JSON, so a custom modifier can read them back,
 * and every other value becomes what the host makes of it.
 *
 * `undefined` answers "this is not a value" — for a value nobody passed, and
 * for one no conversion can describe. Both fall through to the fallback chain,
 * so nothing raises out of `resolve`.
 */
const text = (value: any): string | undefined => {
  if (value === undefined) return undefined;

  try {
    if (isPlainObject(value) || Array.isArray(value)) {
      const json = JSON.stringify(value);

      return typeof json === 'string' ? json : undefined;
    }

    return String(value);
  } catch {
    return undefined;
  }
};

// Reserved by the payload for a value's own configuration.
const WRAPPED = ['value', 'default', 'props'];

const isWrapped = (value: any) => {
  if (!isPlainObject(value)) return false;

  const keys = ownKeys(value);

  return !!keys.length && keys.every((key) => WRAPPED.includes(key));
};

// Layers of `props` compose the way the parser's own defaults and the call's
// already do: each names what it overrides and leaves the rest standing.
const mergeProps = (base: any, override: any) => {
  if (!isPlainObject(override)) return base;

  return mergeLayer(base, override, (from, to) => isPlainObject(to) ? mergeLayer(from, to) : to);
};

const placeholders: Interpolate = ({ value: message, props, payload, parserOptions, locale, key: messageKey }) => {
  const customModifiers: Modifier.CustomModifiers | undefined = ownValue(parserOptions, 'customModifiers');
  const onReport: Parser.OnReport | undefined = ownValue(parserOptions, 'onReport');
  const modifiers = mergeLayer(defaultModifiers, customModifiers);
  const modifierKeys = Object.keys(modifiers);

  const resolvePlaceholder = (placeholder: string) => {
    const [declaration, ...declaredOptions] = split(placeholder.slice(2, -2), ';');
    const [declaredKey, ...declaredModifier] = split(declaration, ':');

    const declaredName = trim(declaredKey);
    const key = declaredName ? unesc(declaredName) as keyof Parser.Payload : undefined;
    const entry = ownValue(payload, key);
    // The payload's root `default` is the fallback itself, never configuration.
    const wrapper = key !== 'default' && isWrapped(entry) ? entry : undefined;
    const value = wrapper ? ownValue(wrapper, 'value') : entry;

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

    // A value nobody passed is not a defect; a value that cannot become text is.
    const payloadText = (declared: any) => {
      const output = text(declared);

      if (declared !== undefined && output === undefined) report('unserializable-value', placeholder, messageKey, onReport);

      return output;
    };

    const valueText = payloadText(value);

    let resolvedDefault: string | undefined;

    // The wrapper speaks for its own value, the payload for every key it does
    // not carry, and the message only for what neither of them says. Converting
    // one costs a full serialization, so a link waits for the one before it to
    // come back empty, and the chain waits for a reader.
    const defaultText = () => {
      resolvedDefault ??= [ownValue(wrapper, 'default'), ownValue(payload, 'default')]
        .reduce<string | undefined>((output, candidate) => output ?? payloadText(candidate), undefined) ?? inlineDefault ?? '';

      return resolvedDefault;
    };

    // A modifier answers to its name, not to the spelling a message needed to
    // write it: an escape is how a name carrying a reserved character reaches
    // the parser, the way a key's and an option key's do.
    const modifierKey = unesc(trim(declaredModifier.join(':')));
    const hasModifier = !!modifierKey;

    // A modifier nobody registered is a defect in the message, not a selection:
    // running `eq` in its place would render a plausible answer to a question the
    // message never asked.
    if (hasModifier && !modifierKeys.includes(modifierKey)) {
      report('unknown-modifier', placeholder, messageKey, onReport);

      return defaultText();
    }

    // An absent value is nothing to compare against, whatever the modifier
    // asks: the placeholder takes the fallback chain rather than measuring the
    // host's own word for absence.
    if (valueText === undefined) return defaultText();

    if (!hasModifier && !options.length) return valueText;

    const modifier = modifiers[(hasModifier ? modifierKey : 'eq') as keyof typeof modifiers];

    // Fail soft: a modifier that raises resolves to the fallback chain, never out of `resolve`.
    try {
      const modifierProps = mergeProps(props, ownValue(wrapper, 'props'));

      // A modifier answers with a host value like any other, so it becomes text
      // by the conversion a payload entry does: an object it built stays
      // structured instead of collapsing to the host's own word for an object.
      // An answer no conversion can describe is not an answer, and neither is
      // nothing, so the placeholder takes the fallback chain — the way it does
      // for a value that is not a value.
      // The default reaches the modifier as a property it reads, not as work
      // done before it was called: a modifier that never asks leaves the chain
      // unresolved, so a link nobody consulted is never described as missing.
      const input = { value: valueText, options, props: modifierProps, get defaultValue() { return defaultText(); }, locale, parserOptions };

      return text(modifier(input)) ?? defaultText();
    } catch {
      return defaultText();
    }
  };

  // A pass already past the output limit is discarded whole, and what follows
  // it can shrink to nothing but no further, so resolving the rest only builds
  // text nobody reads — and a value that multiplies its own placeholder builds
  // text no string can hold.
  const source = `${message}`;
  const parts: string[] = [];
  let growth = 0;
  let from = 0;

  for (let match = nextPlaceholder(source, from); match; match = nextPlaceholder(source, from)) {
    const [open, end] = match;
    const placeholder = source.slice(open, end);
    const resolved = open + growth > MAX_INTERPOLATION_LENGTH ? placeholder : resolvePlaceholder(placeholder);

    parts.push(source.slice(from, open), resolved);

    growth += resolved.length - placeholder.length;
    from = end;
  }

  return [...parts, source.slice(from)].join('');
};

const MAX_INTERPOLATION_PASSES = 10;

const MAX_INTERPOLATION_LENGTH = 100000;

const MAX_REPORTED_LENGTH = 120;

// `JSON.stringify` leaves a terminator it has no short escape for raw, so
// every terminator the format holds is escaped again on top of it. The ones
// it did escape are two characters by then and no longer match.
const excerpt = (value: string) => JSON.stringify(value.length > MAX_REPORTED_LENGTH ? `${value.slice(0, MAX_REPORTED_LENGTH)}...` : value).slice(1, -1).replace(EVERY_TERMINATOR, unicodeEscape);

const REPORT_MESSAGES: Record<Report['code'], string> = {
  'unknown-modifier': 'A placeholder named a modifier this parser does not know.',
  'unserializable-value': 'A payload value could not become text, so resolution read it as missing.',
  'pass-limit': `Interpolation stopped after ${MAX_INTERPOLATION_PASSES} passes. A payload value probably references its own placeholder.`,
  'output-limit': `Interpolation stopped before exceeding ${MAX_INTERPOLATION_LENGTH} characters. A payload value probably multiplies its own placeholder.`,
};

const REPORT_LIMITS: Partial<Record<Report['code'], number>> = {
  'pass-limit': MAX_INTERPOLATION_PASSES,
  'output-limit': MAX_INTERPOLATION_LENGTH,
};

const report = (code: Report['code'], reported: string, key: Parser.Key | undefined, onReport: Parser.OnReport | undefined) => {
  if (!onReport) return;

  try {
    onReport({ code, message: REPORT_MESSAGES[code], key, limit: REPORT_LIMITS[code], text: excerpt(reported) });
  } catch {
    // Reporting is an observation, not a step of the resolution. A host whose
    // logger fails must still get its message back.
  }
};

const interpolate: Interpolation = ({ value, props, payload, parserOptions, locale, key }) => {
  const onReport: Parser.OnReport | undefined = ownValue(parserOptions, 'onReport');

  let output = value;

  for (let pass = 0; hasPlaceholders(output); pass += 1) {
    if (pass === MAX_INTERPOLATION_PASSES) {
      report('pass-limit', output, key, onReport);

      break;
    }

    const next = placeholders({ value: output, payload, props, parserOptions, locale, key });

    if (next.length > MAX_INTERPOLATION_LENGTH) {
      report('output-limit', output, key, onReport);

      break;
    }

    output = next;
  }

  return text(unesc(output)) ?? '';
};

export const createParser: Parser.Factory = (parserOptions) => ({
  resolve: (message, { payload, props, locale, key } = {}) => {
    // Everything the format carries is text, and the message becomes text
    // before anything reads it rather than after everything has: a host that
    // wrote its message as something else gets it interpolated and unescaped
    // like any other. A link no conversion can describe does not exist, the
    // same way such a value is not a value, so the chain steps past it — a
    // message default that cannot become text must not swallow the key echo.
    const value = text(message) ?? text(ownValue(payload, 'default')) ?? (key === undefined ? '' : key);

    return interpolate({ value, payload, props, parserOptions, locale, key });
  },
});
