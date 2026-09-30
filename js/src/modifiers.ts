import type { Modifier } from './types';
import { AGO_LADDER, formatter, getDateInput, getModifierInput, mergeLayer, ModifierFailure, ownValue } from './utils';

// A selection that matched answers with its own value, empty or not. Only a
// selection that matched nothing falls back, and the fallback is read off the
// config rather than destructured out of it: the chain behind it resolves for a
// reader, and a modifier that matched never became one.
const selected = (option: Modifier.ModifierOption | undefined, config: { defaultValue: string }) => (option ? option.value : config.defaultValue);

// The value is normalized once, ahead of the options: what that costs grows
// with the value, and a comparison's cost adds it to its options rather than
// multiplying the two.
export const eq: Modifier.T = (config) => {
  const { value, options = [] } = config;
  const needle = `${value}`.toLowerCase();

  return selected(options.find(({ key }) => `${key}`.toLowerCase() === needle), config);
};

export const ne: Modifier.T = (config) => {
  const { value, options = [] } = config;
  const needle = `${value}`.toLowerCase();

  return selected(options.find(({ key }) => `${key}`.toLowerCase() !== needle), config);
};

// A numeric comparison reads only the options it can order. A key that is not
// numeric can never be selected by one, and leaving it in the list would leave
// the comparator answering NaN, which sorts as equal and freezes the pairs
// around it. Ordering is done on a copy: the list is the caller's, and the
// format has a comparison leave its order alone.
const ordered = (options: Modifier.ModifierOption[], compare: (a: number, b: number) => number) => options
  .filter(({ key }) => !Number.isNaN(+key))
  .sort((a, b) => compare(+a.key, +b.key));

export const lt: Modifier.T = (config) => {
  const { value, options = [] } = config;
  const input = +value;

  return selected(ordered(options, (a, b) => a - b).find(({ key }) => input < +key), config);
};

export const gt: Modifier.T = (config) => {
  const { value, options = [] } = config;
  const input = +value;

  return selected(ordered(options, (a, b) => b - a).find(({ key }) => input > +key), config);
};

// The equality leg answers first and the strict leg second, and each stays
// unread until the one before it comes back empty. The config is handed over
// key by key rather than spread: a spread reads every property it copies, and
// the default is the one property reading costs something.
export const lte: Modifier.T = (config) => eq({ value: config.value, props: config.props, options: config.options, get defaultValue() { return lt(config); } });

export const gte: Modifier.T = (config) => eq({ value: config.value, props: config.props, options: config.options, get defaultValue() { return gt(config); } });

// A value the modifier's own input test rejects is one it cannot format, and a
// modifier that cannot answer says so by raising: the parser reports that and
// resolves the placeholder to the fallback chain.
const formattable = (input: number | undefined) => {
  if (input === undefined) throw new ModifierFailure('failed-modifier');

  return input;
};

// Two fraction digits is what `number` shows when nobody named a maximum: a
// default, not a cap. `Intl` widens its own default maximum to reach a larger
// minimum, and this default widens the same way — held at two over a layer's
// `minimumFractionDigits`, it would contradict it, `Intl` would raise, and the
// number would resolve to a fallback nobody asked for.
const shownDigits = (props: object) => {
  const minimum = Number(ownValue(props, 'minimumFractionDigits')) || 0;
  const maximumFractionDigits = stated(ownValue(props, 'maximumFractionDigits'), Math.max(minimum, 2));

  return mergeLayer(props, { maximumFractionDigits });
};

export const number: Modifier.T<Modifier.NumberProperties> = (config) => {
  const { value, props, locale = '' } = config;

  if (!locale) throw new ModifierFailure('missing-locale');

  const input = formattable(getModifierInput(value));

  return formatter(Intl.NumberFormat, locale, shownDigits(props)).format(input);
};

export const date: Modifier.T<Modifier.DateProperties> = (config) => {
  const { value, props, locale = '' } = config;

  if (!locale) throw new ModifierFailure('missing-locale');

  const input = formattable(getDateInput(value));

  return formatter(Intl.DateTimeFormat, locale, mergeLayer(props, undefined)).format(input);
};

// A property a layer holds the host's null under is one the layer names: null
// is a value, like zero or the empty string, so it is what the host formatter
// is handed. Only a property no layer names takes the default this module
// states for it.
const stated = (value: any, fallback: any) => value === undefined ? fallback : value;

const testResolution = (defKey: string = '', testKey: string = '') => new RegExp(`^${defKey}s?$`).test(testKey);

// The ladder is the whole of what `ago` counts in, so a format naming anything
// else — a unit `Intl` knows and the ladder does not climb, a rung spelled in
// another case, a typo — is a property the modifier cannot process rather than
// one to climb past on the way to `year`.
const onLadder = (format: string) => format === 'auto' || AGO_LADDER.some(({ key }) => testResolution(key, format));

const findIndex = (currentKey: string) => AGO_LADDER.indexOf(AGO_LADDER.find(({ key }) => testResolution(key, currentKey)) as any);

// A step is rounded on its magnitude and given its sign back. The host's own
// rounding takes a half toward positive infinity, which reads a delta and its
// negation differently: half an hour out climbed to "in 1 hour" while half an
// hour past stayed at "30 minutes ago", and 1.5 hours became "in 2 hours"
// against "1 hour ago".
const step = (value: number) => Math.sign(value) * Math.round(Math.abs(value));

const agoFormat = (millis: number, resolution?: Intl.RelativeTimeFormatUnit | 'auto'): [number, Intl.RelativeTimeFormatUnit] => AGO_LADDER.reduce(([value, currentKey], { key, multiplier }, index) => {
  if (testResolution(currentKey, resolution)) return [value, currentKey];

  if (!currentKey || index === findIndex(currentKey) + 1) {
    const output = step(value / multiplier);

    if (!currentKey || Math.abs(output) >= 1 || resolution !== 'auto') return [output, key];
  }

  return [value, currentKey];
}, [millis, '' as Intl.RelativeTimeFormatUnit]);

export const ago: Modifier.T<Modifier.AgoProperties> = (config) => {
  const { value, locale = '', props } = config;

  if (!locale) throw new ModifierFailure('missing-locale');

  const input = formattable(getModifierInput(value));

  const numeric = stated(ownValue(props, 'numeric'), 'auto');
  const format = stated(ownValue(props, 'format'), 'auto');

  if (!onLadder(format)) throw new ModifierFailure('failed-modifier');

  const formatParams = agoFormat(input, format);

  return formatter(Intl.RelativeTimeFormat, locale, mergeLayer(props, { numeric })).format(...formatParams);
};

// A plural selection selects by the category the locale's rules put a number
// in. A key that is a number selects for the value it equals before any
// category is asked for, wherever it is written, so the host is asked only
// where none did. The rule type is what the modifier is rather than a property
// it layers, so it is pinned over every layer the way `currency`'s style is.
const byCategory = (type: Intl.PluralRuleType, properties: (props: object) => object): Modifier.T => (config) => {
  const { value, options = [], props, locale = '' } = config;

  if (!locale) throw new ModifierFailure('missing-locale');

  const input = formattable(getModifierInput(value));

  // An ordinal of a fraction names no position, and the ordinal rules are
  // written for whole numbers: what they make of one differs from locale to
  // locale, so a fraction is a value this modifier cannot take.
  if (type === 'ordinal' && !Number.isInteger(input)) throw new ModifierFailure('failed-modifier');

  const exact = options.find(({ key }) => getModifierInput(key) === input);

  if (exact) return exact.value;

  const category = formatter(Intl.PluralRules, locale, mergeLayer(properties(props), { type })).select(input);

  return selected(options.find(({ key }) => key === category), config);
};

// A count and the word that agrees with it are two placeholders, so `plural`
// takes its category from the number `number` would show: the digits it was
// handed are `number`'s beneath its own, and they widen the way `number`'s do.
export const plural: Modifier.T<Modifier.PluralProperties> = byCategory('cardinal', shownDigits);

// An ordinal takes integers, which show no fraction, so there is nothing of
// `number`'s for it to agree with.
export const ordinal: Modifier.T<Modifier.OrdinalProperties> = byCategory('ordinal', (props) => props);

export const currency: Modifier.T<Modifier.CurrencyProperties> = (config) => {
  const { value, locale = '', props } = config;

  if (!locale) throw new ModifierFailure('missing-locale');

  const amount = formattable(getModifierInput(value));
  const input = formattable(getModifierInput(amount * stated(ownValue(props, 'ratio'), 1)));

  // The currency style is what this modifier is, not one of the options it
  // layers: a layer naming another style asks it to stop being the modifier the
  // message named, so the style is pinned over every layer, the wrapper's
  // included.
  return formatter(Intl.NumberFormat, locale, mergeLayer(props, { style: 'currency' })).format(input);
};
