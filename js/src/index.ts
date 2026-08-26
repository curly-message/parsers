import * as defaultModifiers from './modifiers';
import type { Parser, Modifier, Interpolate, Interpolation, Locale, Report } from './types';

export type { Parser, Modifier, Locale, Report };

const hasPlaceholders = (value: any) => typeof value === 'string' && /{{(?:(?!{{|}}).)+}}/.test(value);

const unesc = (value: any) => typeof value === 'string' ? value.replace(/\\(?=:|;|{|})/g, '') : value;

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

const placeholders: Interpolate = ({ value: text, props, payload, parserOptions, locale }) => `${text}`.replace(/{{(?:\s*(?!{{|}})\S(?:(?:(?!{{|}})[^\n\r\u2028\u2029])*(?!{{|}})\S)?\s*|[\n\r\u2028\u2029]*[^\S\n\r\u2028\u2029]\s*)}}/g, (placeholder) => {
  const [escapedKey] = placeholder.match(/(?!{|\s)(?:\\[:;]|\\(?![:;])|[^:;\\\n\r\u2028\u2029])*?(?:\\[:;]|\\(?![:;])|[^:;\s\\])(?=\s*(?:[:;]|}}$))/) || [];
  const key = escapedKey === undefined ? undefined : unesc(escapedKey) as keyof Parser.Payload;
  const value = ownValue(payload, key);

  const [, inlineDefault] = placeholder.match(/{{(?:[^\\]|\\;|\\(?!;))*?;\s*default\s*:\s*((?:\\[:;]|[^\s:;])(?:\\[:;]|\\(?![:;])|[^;\\])*?)(?=;|}}$)/i) || [];
  const declaredDefault = inlineDefault === undefined ? ownValue(payload, 'default') : inlineDefault;
  const defaultValue = declaredDefault === undefined ? '' : stringify(declaredDefault);

  let [, modifierKey = ''] = placeholder.match(/{{(?:[^;\\]|\\;|\\(?!;))*(?:\\;|[^\\\n\r\u2028\u2029]):\s*(?!\s)((?:\\;|[^;\s])(?:(?:\\;|[^;])*(?:\\;|[^;\s]))?)(?=\s*(?:[;]|}}$))/i) || [];

  if (value === undefined && modifierKey !== 'ne') return defaultValue;

  const hasModifier = !!modifierKey;

  const { customModifiers } = parserOptions || {};
  const modifiers = { ...defaultModifiers, ...(customModifiers || {}) };

  modifierKey = (Object.keys(modifiers).includes(modifierKey) ? modifierKey : 'eq');

  const modifier = modifiers[modifierKey as keyof typeof modifiers];
  const options = (
    placeholder.match(/(?:\\[;]|[^\s:;{}])(?:(?:[^;]|\\[;])*[^:;}])?/gi) as RegExpMatchArray || []
  ).reduce(
    (acc, option, i) => {
      // NOTE: First item is a placeholder and modifier
      if (i > 0) {
        const parts = option.split(/(?<!\\):/);
        const optionKey = unesc(parts[0].trim());
        const optionValue = parts[parts.length - 1].trimStart();

        if (optionKey && optionKey !== 'default' && optionValue) acc.push({ key: optionKey, value: optionValue });
      }

      return acc;
    }, [] as Modifier.ModifierOption[],
  );

  if (!hasModifier && !options.length) return stringify(value, defaultValue);

  // Fail soft: a modifier that raises resolves to the fallback chain, never out of `resolve`.
  try {
    return String(modifier({ value, options, props, defaultValue, locale, parserOptions }));
  } catch {
    return defaultValue;
  }
});

const MAX_INTERPOLATION_PASSES = 10;

const MAX_INTERPOLATION_LENGTH = 100000;

const MAX_REPORTED_LENGTH = 120;

// `JSON.stringify` escapes every line terminator except these two.
const excerpt = (value: string) => JSON.stringify(value.length > MAX_REPORTED_LENGTH ? `${value.slice(0, MAX_REPORTED_LENGTH)}...` : value).slice(1, -1).replace(/[\u2028\u2029]/g, (separator) => `\\u${separator.charCodeAt(0).toString(16)}`);

const REPORT_MESSAGES: Record<Report['code'], string> = {
  'pass-limit': `Interpolation stopped after ${MAX_INTERPOLATION_PASSES} passes. A payload value probably references its own placeholder.`,
  'output-limit': `Interpolation stopped before exceeding ${MAX_INTERPOLATION_LENGTH} characters. A payload value probably multiplies its own placeholder.`,
};

const REPORT_LIMITS: Record<Report['code'], number> = {
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

    const next = placeholders({ value: output, payload, props, parserOptions, locale });

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