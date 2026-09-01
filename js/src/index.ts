import * as defaultModifiers from './modifiers';
import type { Parser, Modifier, Conversions, Interpolate, Interpolation, Locale, Report } from './types';
import { isBlank, LINE_TERM, mergeLayer, ownKeys, ownModifiers, ownValue, unicodeEscape } from './utils';

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

// Serialization follows a shared reference again every time it meets one, so a
// value holding twenty-five objects — each of twenty-four levels naming the
// same child twice — serializes to sixteen million leaves with no cycle for the
// `catch` to find. The budget is what one conversion may spend: a value
// visiting more nodes than a resolvable output can hold is one no conversion
// describes, which is what a cycle and a `toJSON` answering nothing are too.
const serialize = (value: any): string | undefined => {
  let budget = MAX_INTERPOLATION_LENGTH;

  try {
    const json = JSON.stringify(value, (_, entry) => {
      budget -= 1;

      if (budget < 0) throw new RangeError('The value visits more nodes than a resolvable output can hold.');

      return entry;
    });

    return typeof json === 'string' ? json : undefined;
  } catch {
    return undefined;
  }
};

const convert = (value: any): string | undefined => {
  // Classifying a value reads it, and a value the host will not describe raises
  // at that read as readily as at its coercion.
  try {
    if (!isPlainObject(value) && !Array.isArray(value)) return String(value);
  } catch {
    return undefined;
  }

  return serialize(value);
};

/**
 * The text a value resolves to. Everything the format carries is text: a plain
 * object and an array become JSON, so a custom modifier can read them back,
 * and every other value becomes what the host makes of it.
 *
 * `undefined` answers "this is not a value" — for a value nobody passed, and
 * for one no conversion can describe. Both fall through to the fallback chain,
 * so nothing raises out of `resolve`.
 *
 * Converting is what costs, and a value is read once for every placeholder that
 * names it, so `conversions` records what each value came out as, the answer
 * that none describes it included, and one resolution converts one value once.
 */
const text = (value: any, conversions: Conversions): string | undefined => {
  if (value === undefined) return undefined;

  // A primitive's conversion runs no host code and cannot answer twice over, so
  // recording one buys nothing and costs an entry per distinct value.
  const carries = value !== null && (typeof value === 'object' || typeof value === 'function');

  if (!carries) return convert(value);

  if (!conversions.has(value)) conversions.set(value, convert(value));

  return conversions.get(value);
};

// A value nobody passed is not a defect; a value that cannot become text is.
// Both answer nothing, so the one that is a defect has to say so on the way
// past, and every chain the format resolves reads its links through here. The
// conversion behind it runs once for a value; the report does not, because each
// link that finds nothing is a defect of its own.
const describedText = (declared: any, onUndescribed: () => void, conversions: Conversions) => {
  const output = text(declared, conversions);

  if (declared !== undefined && output === undefined) onUndescribed();

  return output;
};

// Reserved by the payload for a value's own configuration.
const WRAPPED = ['value', 'default', 'props'];

const isWrapped = (value: any) => {
  if (!isPlainObject(value)) return false;

  const keys = ownKeys(value);

  return !!keys.length && keys.every((key) => WRAPPED.includes(key));
};

// A configuration layer is anything carrying entries to read. `isPlainObject`
// answers which conversion describes a value better, which is a question about
// values; a layer is read for its own entries and never converted, so a
// prototype it happens to carry decides nothing about how it composes.
const isLayer = (value: any) => !!value && typeof value === 'object';

// A `props` layer copied down to the objects it names. What a modifier
// receives is the parser's own object, so a modifier that writes into what it
// was handed reaches neither the next placeholder nor the caller.
const ownProps = (layer: any) => {
  const output: Record<string, any> = Object.create(null);

  ownKeys(layer).forEach((name) => {
    const value = ownValue(layer, name);

    output[name] = isLayer(value) ? mergeLayer(value, undefined) : value;
  });

  return { ...output };
};

// Layers of `props` compose the way the parser's own defaults and the call's
// already do: each names what it overrides and leaves the rest standing.
const mergeProps = (base: any, override: any) => {
  if (!isLayer(override)) return isLayer(base) ? ownProps(base) : base;

  return ownProps(mergeLayer(base, override, (from, to) => isLayer(to) ? mergeLayer(from, to) : to));
};

const placeholders: Interpolate = ({ value: message, props, payload, parserOptions, locale, key: messageKey, conversions }) => {
  const customModifiers: Modifier.CustomModifiers | undefined = ownValue(parserOptions, 'customModifiers');
  const onReport: Parser.OnReport | undefined = ownValue(parserOptions, 'onReport');
  // The modifier module's exports are the registry a host's table composes
  // with, and each layer contributes the modifiers it holds and nothing else:
  // an entry that cannot be called is not one a message can name and not one
  // that shadows the name it would replace. Filtered after the merge instead,
  // a host's bad entry would take the built-in down with it.
  const modifiers = mergeLayer(ownModifiers(defaultModifiers), ownModifiers(customModifiers));
  const modifierKeys = Object.keys(modifiers);

  const resolvePlaceholder = (placeholder: string) => {
    const [declaration, ...declaredOptions] = split(placeholder.slice(2, -2), ';');
    const [declaredKey, ...declaredModifier] = split(declaration, ':');

    const declaredName = trim(declaredKey);
    const key = declaredName ? unesc(declaredName) as keyof Parser.Payload : undefined;
    // A link that refuses to be read is not a link nobody passed. `ownValue`
    // answers nothing either way, because resolution must not throw; the
    // difference between the two is what a report is for.
    const raised = () => report('unserializable-value', placeholder, messageKey, onReport);
    const entry = ownValue(payload, key, raised);
    // The payload's root `default` is the fallback itself, never configuration.
    const wrapper = key !== 'default' && isWrapped(entry) ? entry : undefined;
    const value = wrapper ? ownValue(wrapper, 'value', raised) : entry;

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

    const payloadText = (declared: any) => describedText(declared, raised, conversions);

    const valueText = payloadText(value);

    let resolvedDefault: string | undefined;

    // A placeholder can name `default` itself, and then the chain's payload
    // link is the entry it has already read as its value. One entry is read
    // once: reading it again would describe a single value that cannot become
    // text as two.
    const payloadDefault = key === 'default' ? () => valueText : () => payloadText(ownValue(payload, 'default', raised));

    // The wrapper speaks for its own value, the payload for every key it does
    // not carry, and the message only for what neither of them says. Converting
    // one costs a full serialization, so a link waits for the one before it to
    // come back empty, and the chain waits for a reader.
    const defaultText = () => {
      resolvedDefault ??= [() => payloadText(ownValue(wrapper, 'default', raised)), payloadDefault]
        .reduce<string | undefined>((output, read) => output ?? read(), undefined) ?? inlineDefault ?? '';

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
    // Containment is what keeps that failure out of the caller's render path,
    // and not a reason for the caller to hear nothing about it.
    try {
      const modifierProps = mergeProps(props, ownValue(wrapper, 'props'));

      // A modifier answers with a host value like any other, so it becomes text
      // by the conversion a payload entry does: an object it built stays
      // structured instead of collapsing to the host's own word for an object.
      // An answer no conversion can describe is not an answer, and neither is
      // nothing, so the placeholder takes the fallback chain — the whole of the
      // treatment a value that is not a value gets, the report included, and
      // an answer that is nothing is absent rather than undescribable.
      // The default reaches the modifier as a property it reads, not as work
      // done before it was called: a modifier that never asks leaves the chain
      // unresolved, so a link nobody consulted is never described as missing.
      const input = { value: valueText, options, props: modifierProps, get defaultValue() { return defaultText(); }, locale, parserOptions };

      return describedText(modifier(input), raised, conversions) ?? defaultText();
    } catch {
      report('failed-modifier', placeholder, messageKey, onReport);

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
  'failed-modifier': 'A modifier could not produce a result, so the placeholder took its fallback chain.',
  'unserializable-value': 'A value could not become text, so resolution read it as missing.',
  'pass-limit': `Interpolation stopped after ${MAX_INTERPOLATION_PASSES} passes. A payload value probably references its own placeholder.`,
  'output-limit': `Interpolation stopped before exceeding ${MAX_INTERPOLATION_LENGTH} characters. A payload value probably multiplies its own placeholder.`,
};

// A code that reached no limit names one all the same, because a table read by
// a key it does not carry answers for its prototype, and a report would then
// carry out whatever somebody else had written there.
const REPORT_LIMITS: Record<Report['code'], number | undefined> = {
  'unknown-modifier': undefined,
  'failed-modifier': undefined,
  'unserializable-value': undefined,
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

const interpolate: Interpolation = ({ value, props, payload, parserOptions, locale, key, conversions }) => {
  const onReport: Parser.OnReport | undefined = ownValue(parserOptions, 'onReport');

  let output = value;

  for (let pass = 0; hasPlaceholders(output); pass += 1) {
    if (pass === MAX_INTERPOLATION_PASSES) {
      report('pass-limit', output, key, onReport);

      break;
    }

    const next = placeholders({ value: output, payload, props, parserOptions, locale, key, conversions });

    if (next.length > MAX_INTERPOLATION_LENGTH) {
      report('output-limit', output, key, onReport);

      break;
    }

    output = next;
  }

  return text(unesc(output), conversions) ?? '';
};

export const createParser: Parser.Factory = (parserOptions) => ({
  resolve: (message, context) => {
    // The context is caller-supplied like the payload inside it, and is read
    // the same way: own entries only. A prototype somebody else wrote to is
    // not a context a caller passed, and a caller that passed `null` for one
    // is a caller that passed none.
    const payload: Parser.Payload | undefined = ownValue(context, 'payload');
    const props: Modifier.Props | undefined = ownValue(context, 'props');
    const locale: Locale | undefined = ownValue(context, 'locale');
    const key: Parser.Key | undefined = ownValue(context, 'key');

    const onReport: Parser.OnReport | undefined = ownValue(parserOptions, 'onReport');
    // A report about the chain a message resolves through names no placeholder,
    // and the link it is about is one nothing describes, so it carries no
    // excerpt: the key is what says which message went looking.
    const raised = () => report('unserializable-value', '', key, onReport);

    // One value converts once, however many placeholders read it: the walk is
    // the costly step. The call is the scope — a payload the host mutates
    // between two of them must not be answered with the older text.
    const conversions: Conversions = new Map();

    // Everything the format carries is text, and the message becomes text
    // before anything reads it rather than after everything has: a host that
    // wrote its message as something else gets it interpolated and unescaped
    // like any other. A link no conversion can describe does not exist, the
    // same way such a value is not a value, so the chain steps past it — a
    // message default that cannot become text must not swallow the key echo.
    // Stepping past the payload's link is reported, because that link is a
    // payload value like any other; stepping past the message is not, because
    // a message nothing describes is a message nobody wrote.
    const value = text(message, conversions) ?? describedText(ownValue(payload, 'default', raised), raised, conversions);

    // What is left when the chain runs out is not a message the format resolves
    // over: it is the format naming the message that went looking. A value, an
    // option value, an inline default and a payload `default` are what may
    // carry placeholders; a key is not one of them, so it leaves as the caller
    // spelled it, neither resolved nor unescaped. It still becomes text,
    // because `resolve` answers with text, and a caller that named no key has
    // nothing to echo.
    if (value === undefined) return text(key, conversions) ?? '';

    return interpolate({ value, payload, props, parserOptions, locale, key, conversions });
  },
});
