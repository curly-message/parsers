import * as defaultModifiers from './modifiers';
import type { Parser, Modifier, Interpolate, Interpolation, Locale, Report } from './types';

export type { Parser, Modifier, Locale, Report };

const hasPlaceholders = (value: any) => typeof value === 'string' && /{{(?:(?!{{|}}).)+}}/.test(value);

// The syntax reserves a colon, a semicolon, either brace, a backslash and
// whitespace. A backslash writes any of them as text; before anything else it
// is text itself, so a Windows path and a regular expression survive as typed.
const RESERVED = /[:;{}\\\s]/;

const unesc = (value: any) => typeof value === 'string' ? value.replace(/\\([\s\S])/g, (sequence, character) => RESERVED.test(character) ? character : sequence) : value;

// Whitespace an escape sequence claims is text, not padding around it.
const trim = (value: string) => {
  let start = -1;
  let end = 0;

  for (let index = 0; index < value.length; index += 1) {
    const escaped = value[index] === '\\' && index + 1 < value.length;

    if (!escaped && /\s/.test(value[index])) continue;

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

const ownValue = (target: any, key?: PropertyKey) => {
  try {
    return key !== undefined && !!target && Object.prototype.hasOwnProperty.call(target, key) ? target[key] : undefined;
  } catch {
    return undefined;
  }
};

// Fail soft: a value that cannot become text resolves to the fallback chain, never out of `resolve`.
const stringify = (value: any, fallback = '') => {
  try {
    return String(value);
  } catch {
    return fallback;
  }
};

const placeholders: Interpolate = ({ value: text, props, payload, parserOptions, locale, key: messageKey }) => {
  const { customModifiers, onReport } = parserOptions || {};
  const modifiers = { ...defaultModifiers, ...(customModifiers || {}) };
  const modifierKeys = Object.keys(modifiers);

  return `${text}`.replace(/{{(?:\s*(?!{{|}})\S(?:(?:(?!{{|}})[^\n\r\u2028\u2029])*(?!{{|}})\S)?\s*|[\n\r\u2028\u2029]*[^\S\n\r\u2028\u2029]\s*)}}/g, (placeholder) => {
    const [declaration, ...declaredOptions] = split(placeholder.slice(2, -2), ';');
    const [declaredKey, ...declaredModifier] = split(declaration, ':');

    const declaredName = trim(declaredKey);
    const key = declaredName ? unesc(declaredName) as keyof Parser.Payload : undefined;
    const value = ownValue(payload, key);

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

      if (inlineDefault === undefined && optionKey.toLowerCase() === 'default') inlineDefault = optionValue;

      if (optionKey !== 'default') options.push({ key: optionKey, value: optionValue });
    });

    const payloadDefault = ownValue(payload, 'default');
    const declaredDefault = payloadDefault === undefined ? inlineDefault : payloadDefault;
    const defaultValue = declaredDefault === undefined ? '' : declaredDefault;
    const defaultText = stringify(defaultValue);

    const modifierKey = trim(declaredModifier.join(':'));
    const hasModifier = !!modifierKey;

    // A modifier nobody registered is a defect in the message, not a selection:
    // running `eq` in its place would render a plausible answer to a question the
    // message never asked.
    if (hasModifier && !modifierKeys.includes(modifierKey)) {
      report('unknown-modifier', placeholder, messageKey, onReport);

      return defaultText;
    }

    if (value === undefined && modifierKey !== 'ne') return defaultText;

    if (!hasModifier && !options.length) return stringify(value, defaultText);

    const modifier = modifiers[(hasModifier ? modifierKey : 'eq') as keyof typeof modifiers];

    // Fail soft: a modifier that raises resolves to the fallback chain, never out of `resolve`.
    try {
      return String(modifier({ value, options, props, defaultValue, locale, parserOptions }));
    } catch {
      return defaultText;
    }
  });
};

const MAX_INTERPOLATION_PASSES = 10;

const MAX_INTERPOLATION_LENGTH = 100000;

const MAX_REPORTED_LENGTH = 120;

// `JSON.stringify` escapes every line terminator except these two.
const excerpt = (value: string) => JSON.stringify(value.length > MAX_REPORTED_LENGTH ? `${value.slice(0, MAX_REPORTED_LENGTH)}...` : value).slice(1, -1).replace(/[\u2028\u2029]/g, (separator) => `\\u${separator.charCodeAt(0).toString(16)}`);

const REPORT_MESSAGES: Record<Report['code'], string> = {
  'unknown-modifier': 'A placeholder named a modifier this parser does not know.',
  'pass-limit': `Interpolation stopped after ${MAX_INTERPOLATION_PASSES} passes. A payload value probably references its own placeholder.`,
  'output-limit': `Interpolation stopped before exceeding ${MAX_INTERPOLATION_LENGTH} characters. A payload value probably multiplies its own placeholder.`,
};

const REPORT_LIMITS: Partial<Record<Report['code'], number>> = {
  'pass-limit': MAX_INTERPOLATION_PASSES,
  'output-limit': MAX_INTERPOLATION_LENGTH,
};

const report = (code: Report['code'], text: string, key: Parser.Key | undefined, onReport: Parser.OnReport | undefined) => {
  if (!onReport) return;

  onReport({ code, message: REPORT_MESSAGES[code], key, limit: REPORT_LIMITS[code], text: excerpt(text) });
};

const interpolate: Interpolation = ({ value, props, payload, parserOptions, locale, key }) => {
  const { onReport } = parserOptions || {};

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

  return stringify(unesc(output));
};

export const createParser: Parser.Factory = (parserOptions) => ({
  resolve: (message, { payload, props, locale, key } = {}) => {
    let value = message;

    if (value === undefined) {
      value = ownValue(payload, 'default');
    }

    if (value === undefined) {
      value = key === undefined ? '' : key;
    }

    return interpolate({ value, payload, props, parserOptions, locale, key });
  },
});
