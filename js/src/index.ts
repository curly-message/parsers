import * as defaultModifiers from './modifiers';
import type { Cst, Parser, Modifier, Conversions, Interpolation, Locale, Report, Wrappers } from './types';
import { EVERY_TERMINATOR, failureCode, mergeLayer, ownKeys, ownModifiers, ownValue, parsePlaceholder, scanner, unesc, unicodeEscape } from './utils';

export type { Cst, Parser, Modifier, Locale, Report };

// The scanner is a named export of this entry rather than a subpath of its
// own: the package is ESM and declares `sideEffects: false`, so a bundle that
// never reaches it drops it, and a build-time caller is not bundled at all.
export { createExtractor } from './extract';
export { cst } from './cst';

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
  let budget = MAX_CONVERSION_NODES;

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

  // Recording pays where converting costs. An object or a function runs host
  // code, which can answer differently the second time as readily as it can
  // cost twice; a bigint runs none, but the digits it converts to are work the
  // engine charges for again on every read. Every other primitive converts for
  // free, so an entry for one buys nothing and costs an entry per distinct
  // value.
  const recorded = value !== null && (typeof value === 'object' || typeof value === 'function' || typeof value === 'bigint');

  if (!recorded) return convert(value);

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

// Recognizing a wrapper enumerates the entry, which reads it as a conversion
// does and costs what the entry holds, so the verdict is recorded for the
// resolution the way a conversion is, the answer that the entry refused the
// question included. The report is not: each placeholder that reads a refusing
// entry is a defect of its own.
const isWrapped = (value: any, wrappers: Wrappers, onRaise?: () => void) => {
  if (!isPlainObject(value)) return false;

  if (!wrappers.has(value)) {
    let refused = false;
    const keys = ownKeys(value, () => { refused = true; });

    wrappers.set(value, refused ? undefined : !!keys.length && keys.every((key) => WRAPPED.includes(key)));
  }

  const wrapped = wrappers.get(value);

  if (wrapped === undefined) onRaise?.();

  return !!wrapped;
};

// A configuration layer is anything carrying entries to read. `isPlainObject`
// answers which conversion describes a value better, which is a question about
// values; a layer is read for its own entries and never converted, so neither
// a prototype nor a call signature it happens to carry decides how it
// composes.
const isLayer = (value: any) => !!value && (typeof value === 'object' || typeof value === 'function');

// What a modifier reads is the properties under its own name, so that name is
// what the layers are read for and nothing else: each contributes the entry it
// holds under it, and an entry overrides only the properties it names, so an
// entry that is no layer names none and overrides nothing. What leaves is the
// parser's own copy, so a modifier that writes into what it was handed reaches
// neither the next placeholder nor the caller.
const ownSlice = (layers: any[], name: string, onRaise?: () => void) => layers.reduce((from, layer) => {
  const to = isLayer(layer) ? ownValue(layer, name, onRaise) : undefined;

  return isLayer(to) ? mergeLayer(from, to, onRaise) : from;
}, Object.create(null));

// The names this format defines as comparisons, typed off the registry so a
// name that stops being a built-in stops compiling here. A message that writes
// one has asked for a selection while the format's own modifier answers to the
// name; a host that registered its own has replaced the comparison, and
// whether that modifier needs options is its own business.
const COMPARISONS: Modifier.DefaultKeys[] = ['eq', 'ne', 'lt', 'gt', 'lte', 'gte'];

// Whether the modifier answering to a name is the comparison this format
// defines under it, and not one a host registered in its place.
const isComparison = (name: string, modifiers: Record<string, unknown>) => COMPARISONS.includes(name as Modifier.DefaultKeys) && modifiers[name] === defaultModifiers[name as Modifier.DefaultKeys];

// The modifier module's exports are the registry a host's table composes with.
// They are a constant of the module, so the registry is read off them once.
const builtInModifiers = ownModifiers(defaultModifiers);

const MAX_OUTPUT_LENGTH = 100000;

const MAX_READ_LENGTH = 100000;

const MAX_CONVERSION_NODES = 100000;

const MAX_NESTING = 8;

const MAX_REPORTED_LENGTH = 120;

// A cut that would fall between the halves of a surrogate pair stops one unit
// short, so an excerpt ends on a whole character and not on an escaped half.
const cut = (value: string) => value.slice(0, (value.codePointAt(MAX_REPORTED_LENGTH - 1) ?? 0) > 0xffff ? MAX_REPORTED_LENGTH - 1 : MAX_REPORTED_LENGTH);

// `JSON.stringify` leaves a terminator it has no short escape for raw, so
// every terminator the format holds is escaped again on top of it. The ones
// it did escape are two characters by then and no longer match.
const excerpt = (value: string) => JSON.stringify(value.length > MAX_REPORTED_LENGTH ? `${cut(value)}...` : value).slice(1, -1).replace(EVERY_TERMINATOR, unicodeEscape);

const REPORT_MESSAGES: Record<Report['code'], string> = {
  'unknown-modifier': 'A placeholder named a modifier this parser does not know.',
  'failed-modifier': 'A modifier could not produce a result, so the placeholder took its fallback chain.',
  'missing-options': 'A comparison was given no options to select from, so the placeholder took its fallback chain.',
  'unserializable-value': 'A value could not become text, so resolution read it as missing.',
  'missing-locale': 'A formatting modifier was given no locale, so the placeholder resolved to the empty string.',
  'nesting-limit': `A placeholder was nested deeper than ${MAX_NESTING} levels, so it took its fallback chain.`,
  'output-limit': `A placeholder resolved to text this resolution has no room for, and would have carried the output past ${MAX_OUTPUT_LENGTH} characters, so it resolved to the empty string.`,
  'read-limit': `Resolution read more than ${MAX_READ_LENGTH} characters of value text, so this placeholder resolved to the empty string.`,
};

// A code that reached no limit names one all the same, because a table read by
// a key it does not carry answers for its prototype, and a report would then
// carry out whatever somebody else had written there.
const REPORT_LIMITS: Record<Report['code'], number | undefined> = {
  'unknown-modifier': undefined,
  'failed-modifier': undefined,
  'missing-options': undefined,
  'unserializable-value': undefined,
  'missing-locale': undefined,
  'nesting-limit': MAX_NESTING,
  'output-limit': MAX_OUTPUT_LENGTH,
  'read-limit': MAX_READ_LENGTH,
};

// The axis is a property of the code rather than of the site that reported it,
// so every code names its own here and no report site chooses one. The table
// names them all for the reason the limits do. A modifier that could not
// produce a result was handed the caller's value, props and locale, or is the
// caller's own, so what it reports is the payload's.
const REPORT_ORIGINS: Record<Report['code'], Report['origin']> = {
  'unknown-modifier': 'message',
  'failed-modifier': 'payload',
  'missing-options': 'message',
  'unserializable-value': 'payload',
  'missing-locale': 'payload',
  'nesting-limit': 'message',
  'output-limit': 'limit',
  'read-limit': 'limit',
};

const report = (code: Report['code'], reported: string, id: Parser.Id | undefined, onReport: Parser.OnReport | undefined) => {
  if (!onReport) return;

  try {
    onReport({ code, origin: REPORT_ORIGINS[code], message: REPORT_MESSAGES[code], id, limit: REPORT_LIMITS[code], text: excerpt(reported) });
  } catch {
    // Reporting is an observation, not a step of the resolution. A host whose
    // logger fails must still get its message back.
  }
};

// A message is resolved in one walk (specification, section 5). The message's
// own text is syntax and is scanned for placeholders; what a placeholder
// resolves to is data and is never scanned again, so nesting is what the
// message spells rather than what a payload arranges (section 12).
const interpolate: Interpolation = ({ value: message, props, payload, parserOptions, modifiers, modifierDefaults, onReport, recognizeWrappers, locale, id: messageId, conversions, wrappers }) => {
  const source = `${message}`;
  // One scanner for the walk: a verdict is final, so the span an opening brace
  // derives is settled once however many times the walk asks for it.
  const scan = scanner(source);
  const modifierKeys = Object.keys(modifiers);

  // What the output carries and what the payload is read for are budgets of
  // the resolution rather than of a placeholder: each is spent by what reaches
  // it, and what one placeholder spends is gone for every placeholder after
  // it (section 13).
  let output = MAX_OUTPUT_LENGTH;
  let read = MAX_READ_LENGTH;

  const resolvePlaceholder = (open: number, close: number, depth: number): string => {
    const spelling = source.slice(open, close);
    // A link that refuses to be read is not a link nobody passed. `ownValue`
    // answers nothing either way, because resolution must not throw; the
    // difference between the two is what a report is for.
    const raised = () => report('unserializable-value', spelling, messageId, onReport);

    // The budget is tested before this placeholder reads, so one that found a
    // budget and spent it past the limit resolved all the same, and this is
    // the one that pays for it.
    if (read < 0) {
      report('read-limit', spelling, messageId, onReport);

      return '';
    }

    const { key, modifier: modifierKey, options: segments, inlineDefault } = parsePlaceholder(source, open, close, scan.end);
    const entry = ownValue(payload, key, raised);
    // The payload's root `default` is the fallback itself, never configuration.
    // A caller that turned recognition off passes entries it did not write, so
    // an entry shaped like a wrapper is a value here and converts as one.
    const wrapper = recognizeWrappers && key !== 'default' && isWrapped(entry, wrappers, raised) ? entry : undefined;
    const value = wrapper ? ownValue(wrapper, 'value', raised) : entry;

    // Value text is what a placeholder takes from the payload, and every
    // character of it is spent whether or not any of it reaches the output: a
    // placeholder that reads a long value and selects nothing from it has
    // still done the reading.
    const payloadText = (declared: any) => {
      const declaredText = describedText(declared, raised, conversions);

      if (declaredText !== undefined) read -= declaredText.length;

      return declaredText;
    };

    // An option value is the message's own text, so it is rendered rather than
    // read, and the placeholders it writes resolve one level deeper. A
    // modifier asks for the options it was given when it wants them and may
    // ask twice, so a value is rendered where it is first asked for and not
    // again — and an option the modifier passes over is never rendered at all,
    // so however deeply it nests it costs nothing and reaches no limit.
    const rendered = new Map<[number, number], string>();
    const valueOf = (span: [number, number]) => {
      const known = rendered.get(span);

      if (known !== undefined) return known;

      const resolved = render(span[0], span[1], depth + 1);

      rendered.set(span, resolved);

      return resolved;
    };

    const options = segments.map((segment) => ({ key: segment.key, get value() { return valueOf(segment.value); } }));

    const valueText = payloadText(value);

    let resolvedDefault: string | undefined;

    // A placeholder can name `default` itself, and then the chain's payload
    // link is the entry it has already read as its value. One entry is read
    // once: reading it again would describe a single value that cannot become
    // text as two, and would spend the read budget twice over.
    const payloadDefault = key === 'default' ? () => valueText : () => payloadText(ownValue(payload, 'default', raised));

    // The wrapper speaks for its own value, the payload for every key it does
    // not carry, and the message only for what neither of them says. Converting
    // one costs a full serialization, so a link waits for the one before it to
    // come back empty, and the chain waits for a reader.
    const defaultText = () => {
      resolvedDefault ??= [() => payloadText(ownValue(wrapper, 'default', raised)), payloadDefault]
        .reduce<string | undefined>((chain, link) => chain ?? link(), undefined) ?? (inlineDefault && valueOf(inlineDefault)) ?? '';

      return resolvedDefault;
    };

    // The nesting limit bounds resolution and not derivation: every
    // placeholder's span is settled before the walk starts, and what the limit
    // refuses is resolving this one. It is a defect of the message, so the
    // placeholder takes its fallback chain and the walk carries on.
    if (depth > MAX_NESTING) {
      report('nesting-limit', spelling, messageId, onReport);

      return defaultText();
    }

    const hasModifier = !!modifierKey;

    // A modifier nobody registered is a defect in the message, not a selection:
    // running `eq` in its place would render a plausible answer to a question the
    // message never asked.
    if (hasModifier && !modifierKeys.includes(modifierKey)) {
      report('unknown-modifier', spelling, messageId, onReport);

      return defaultText();
    }

    // A comparison selects among the options a placeholder declares, so one
    // declaring none was asked to select from nothing. It is how the
    // placeholder is written that says so, which is why the report does not
    // wait on the value: a placeholder whose value is absent takes the chain
    // below and is reported all the same. A placeholder naming no key has
    // nothing to compare and is no selection, so a comparison it names was
    // asked nothing. A host that registered its own modifier under the name
    // replaced the comparison, so the placeholder asks the host's modifier
    // and is no selection either.
    if (key !== undefined && !options.length && isComparison(modifierKey, modifiers)) report('missing-options', spelling, messageId, onReport);

    // An absent value is nothing to compare against, whatever the modifier
    // asks: the placeholder takes the fallback chain rather than measuring the
    // host's own word for absence.
    if (valueText === undefined) return defaultText();

    if (!hasModifier && !options.length) return valueText;

    const modifierName = hasModifier ? modifierKey : 'eq';
    const modifier = modifiers[modifierName];

    // Fail soft: a modifier that raises resolves its placeholder, never out of `resolve`.
    // Containment is what keeps that failure out of the caller's render path,
    // and not a reason for the caller to hear nothing about it.
    try {
      // Every layer names the modifiers it configures, and a modifier reads the
      // slice its own name holds rather than the table those layers are: what
      // one modifier is configured with is not what the next reads, and a
      // modifier nobody configured reads an object all the same.
      const modifierProps = ownSlice([modifierDefaults, props, ownValue(wrapper, 'props', raised)], modifierName, raised);

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
      // What the modifier answers with is this placeholder's result rather than
      // text it read, so it is bounded by the output limit and spends nothing
      // of the read budget.
      const input = { value: valueText, options, props: modifierProps, get defaultValue() { return defaultText(); }, locale, parserOptions };

      return describedText(modifier(input), raised, conversions) ?? defaultText();
    } catch (failure) {
      // A built-in modifier that cannot answer says why by raising, the way a
      // host-defined one already does; a raise that says nothing is the
      // failure it has always been.
      const code = failureCode(failure) ?? 'failed-modifier';

      report(code, spelling, messageId, onReport);

      // A locale nobody supplied is the one failure the chain does not answer:
      // a declared default stands in for a value the modifier cannot read,
      // never for the locale it would have formatted in.
      return code === 'missing-locale' ? '' : defaultText();
    }
  };

  // A span of the message: the characters it wrote itself, with their escape
  // sequences removed, and the placeholders it wrote resolved. The whole
  // message is one such span, and so is each option value a modifier asks for.
  const render = (from: number, to: number, depth: number): string => {
    const parts: string[] = [];
    let at = from;

    for (let match = scan.next(at, to); match; match = scan.next(at, to)) {
      const [open, close] = match;

      parts.push(unesc(source.slice(at, open)));

      const resolved = resolvePlaceholder(open, close, depth);

      // Only what the output carries is counted, and a result nested in
      // another reaches the output through the one around it, so the charge is
      // made where the walk meets the message itself and a result is counted
      // once. The message's own text is the caller's and always renders.
      if (depth > 1) parts.push(resolved);
      else if (resolved.length <= output) {
        output -= resolved.length;

        parts.push(resolved);
      } else report('output-limit', source.slice(open, close), messageId, onReport);

      at = close;
    }

    parts.push(unesc(source.slice(at, to)));

    return parts.join('');
  };

  return render(0, source.length, 1);
};

export const createParser: Parser.Factory = (parserOptions) => ({
  resolve: (message, context) => {
    // The context is caller-supplied like the payload inside it, and is read
    // the same way: own entries only. A prototype somebody else wrote to is
    // not a context a caller passed, and a caller that passed `null` for one
    // is a caller that passed none.
    const onReport: Parser.OnReport | undefined = ownValue(parserOptions, 'onReport') ?? undefined;
    // A context entry or an entry of the option bag that refuses to be read is
    // the call's own defect rather than a placeholder's, and the id that would
    // name the message is one of the entries, so the message is what a report
    // of one carries — as text where the caller wrote text, because nothing
    // has converted it yet. Both are the call's own structure, read once for
    // the call: a pass composes over what the call holds rather than asking
    // the bag again.
    const reported = typeof message === 'string' ? message : '';
    const id: Parser.Id | undefined = ownValue(context, 'id', () => report('unserializable-value', reported, undefined, onReport));
    const callRaised = () => report('unserializable-value', reported, id, onReport);
    const payload: Parser.Payload | undefined = ownValue(context, 'payload', callRaised);
    const props: Modifier.Props | undefined = ownValue(context, 'props', callRaised);
    const locale: Locale | undefined = ownValue(context, 'locale', callRaised);
    const customModifiers: Modifier.CustomModifiers | undefined = ownValue(parserOptions, 'customModifiers', callRaised);
    const modifierDefaults: Modifier.Props | undefined = ownValue(parserOptions, 'modifierDefaults', callRaised);
    // Recognition is on where the caller says nothing (specification, section
    // 4.1), so an entry that refuses to be read leaves it where it was and is
    // reported like every other refusal.
    const recognizeWrappers = !!(ownValue(parserOptions, 'recognizeWrappers', callRaised) ?? true);
    // Each layer of the registry contributes the modifiers it holds and nothing
    // else: an entry that cannot be called is not one a message can name and
    // not one that shadows the name it would replace. Filtered after the merge
    // instead, a host's bad entry would take the built-in down with it. A call
    // that holds no table composes nothing over the built-in registry, which
    // nothing writes to.
    const modifiers = customModifiers === undefined ? builtInModifiers : mergeLayer(builtInModifiers, ownModifiers(customModifiers, callRaised));
    // One value converts once, however many placeholders read it: the walk is
    // the costly step. So is one entry asked once whether it configures its
    // value, because asking enumerates it. The call is the scope — a payload
    // the host mutates between two of them must not be answered with the older
    // text.
    const conversions: Conversions = new Map();
    const wrappers: Wrappers = new Map();

    // Everything the format carries is text, and the message becomes text
    // before anything reads it rather than after everything has: a host that
    // wrote its message as something else gets it interpolated and unescaped
    // like any other. A message no conversion can describe is nothing to
    // resolve, the same way such a value is not a value, and that is not a
    // condition to report: a message nothing describes is a message nobody
    // wrote, and nothing behind it is read on its account.
    const value = text(message, conversions);

    if (value === undefined) return '';

    return interpolate({ value, payload, props, parserOptions, modifiers, modifierDefaults, onReport, recognizeWrappers, locale, id, conversions, wrappers });
  },
});
