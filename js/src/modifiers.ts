import type { Modifier } from './types';
import { AGO_LADDER, getDateInput, getModifierInput, mergeLayer, ModifierFailure, ownValue } from './utils';

// A selection that matched answers with its own value, empty or not. Only a
// selection that matched nothing falls back, and the fallback is read off the
// config rather than destructured out of it: the chain behind it resolves for a
// reader, and a modifier that matched never became one.
const selected = (option: Modifier.ModifierOption | undefined, config: { defaultValue: string }) => (option ? option.value : config.defaultValue);

export const eq: Modifier.T = (config) => {
  const { value, options = [] } = config;

  return selected(options.find(
    ({ key }) => `${key}`.toLowerCase() === `${value}`.toLowerCase(),
  ), config);
};

export const ne: Modifier.T = (config) => {
  const { value, options = [] } = config;

  return selected(options.find(
    ({ key }) => `${key}`.toLowerCase() !== `${value}`.toLowerCase(),
  ), config);
};

// A numeric comparison reads only the options it can order. A key that is not
// numeric can never be selected by one, and leaving it in the list would leave
// the comparator answering NaN, which sorts as equal and freezes the pairs
// around it. Ordering is done on a copy, so the caller's list keeps its order
// for the `eq` leg `lte` and `gte` run over it.
const ordered = (options: Modifier.ModifierOption[], compare: (a: number, b: number) => number) => options
  .filter(({ key }) => !Number.isNaN(+key))
  .sort((a, b) => compare(+a.key, +b.key));

export const lt: Modifier.T = (config) => {
  const { value, options = [] } = config;

  return selected(ordered(options, (a, b) => a - b).find(
    ({ key }) => +value < +key,
  ), config);
};

export const gt: Modifier.T = (config) => {
  const { value, options = [] } = config;

  return selected(ordered(options, (a, b) => b - a).find(
    ({ key }) => +value > +key,
  ), config);
};

// The equality leg answers first and the strict leg second, and each stays
// unread until the one before it comes back empty. The config is handed over
// key by key rather than spread: a spread reads every property it copies, and
// the default is the one property reading costs something.
export const lte: Modifier.T = (config) => eq({ value: config.value, options: config.options, get defaultValue() { return lt(config); } });

export const gte: Modifier.T = (config) => eq({ value: config.value, options: config.options, get defaultValue() { return gt(config); } });

// A value the modifier's own input test rejects is one it cannot format, and a
// modifier that cannot answer says so by raising: the parser reports that and
// resolves the placeholder to the fallback chain the modifier used to read for
// itself.
const formattable = (input: number | undefined) => {
  if (input === undefined) throw new ModifierFailure('failed-modifier');

  return input;
};

export const number: Modifier.T<Modifier.NumberProps> = (config) => {
  const { value, props, locale = '' } = config;

  if (!locale) return '';

  const input = formattable(getModifierInput(value));

  // Two fraction digits is what this modifier formats when nobody named a
  // maximum: a default, not a cap. `Intl` widens its own default maximum to
  // reach a larger minimum, and this default widens the same way — held at two
  // over a layer's `minimumFractionDigits`, it would contradict it, `Intl`
  // would raise, and the number would resolve to a fallback nobody asked for.
  const minimum = Number(ownValue(props, 'minimumFractionDigits')) || 0;
  const maximumFractionDigits = ownValue(props, 'maximumFractionDigits') ?? Math.max(minimum, 2);

  return new Intl.NumberFormat(locale, mergeLayer(props, { maximumFractionDigits })).format(input);
};

export const date: Modifier.T<Modifier.DateProps> = (config) => {
  const { value, props, locale = '' } = config;

  if (!locale) return '';

  const input = formattable(getDateInput(value));

  return new Intl.DateTimeFormat(locale, mergeLayer(props, undefined)).format(input);
};

const testResolution = (defKey: string = '', testKey: string = '') => new RegExp(`^${defKey}s?$`).test(testKey);

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

export const ago: Modifier.T<Modifier.AgoProps> = (config) => {
  const { value, locale = '', props } = config;

  if (!locale) return '';

  const input = formattable(getModifierInput(value));

  const numeric = ownValue(props, 'numeric') ?? 'auto';
  const formatParams = agoFormat(input, ownValue(props, 'format') ?? 'auto');

  return new Intl.RelativeTimeFormat(locale, mergeLayer(props, { numeric })).format(...formatParams);
};

export const currency: Modifier.T<Modifier.CurrencyProps> = (config) => {
  const { value, locale = '', props } = config;

  if (!locale) return '';

  const amount = formattable(getModifierInput(value));
  const input = formattable(getModifierInput(amount * (ownValue(props, 'ratio') ?? 1)));

  // The currency style is what this modifier is, not one of the options it
  // layers: a layer naming another style asks it to stop being the modifier the
  // message named. Pinning it against the parser's defaults alone left the call
  // and the wrapper able to render `{{v:currency}}` as a percentage.
  return new Intl.NumberFormat(locale, mergeLayer(props, { style: 'currency' })).format(input);
};
