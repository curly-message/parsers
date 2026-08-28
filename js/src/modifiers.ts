import type { Modifier } from './types';
import { AGO_LADDER, formatOptions, getDateInput, getModifierDefaults, getModifierInput, mergeLayer, ownLayer, ownValue } from './utils';

// A selection that matched answers with its own value, empty or not. Only a
// selection that matched nothing falls back, and the fallback is read off the
// config rather than destructured out of it: the chain behind it resolves for a
// reader, and a modifier that matched never became one.
const selected = (option: Modifier.ModifierOption | undefined, config: { defaultValue?: Modifier.DefaultValue }) => (option ? option.value : config.defaultValue ?? '');

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

export const number: Modifier.T<Modifier.NumberProps> = (config) => {
  const { value, props, locale = '', parserOptions } = config;

  if (!locale) return '';

  const input = getModifierInput(value);

  if (input === undefined) return config.defaultValue ?? '';

  const layered = mergeLayer(getModifierDefaults<Modifier.NumberProps>('number', parserOptions), ownLayer(props, 'number'));
  // Two fraction digits is what this modifier formats when nobody named a
  // maximum: a default, not a cap. `Intl` widens its own default maximum to
  // reach a larger minimum, and this default widens the same way — held at two
  // over a layer's `minimumFractionDigits`, it would contradict it, `Intl`
  // would raise, and the number would resolve to a fallback nobody asked for.
  const minimum = Number(ownValue(layered, 'minimumFractionDigits')) || 0;
  const maximumFractionDigits = ownValue(layered, 'maximumFractionDigits') ?? Math.max(minimum, 2);

  return new Intl.NumberFormat(locale, formatOptions(layered, { maximumFractionDigits })).format(input);
};

export const date: Modifier.T<Modifier.DateProps> = (config) => {
  const { value, props, locale = '', parserOptions } = config;

  if (!locale) return '';

  const input = getDateInput(value);

  if (input === undefined) return config.defaultValue ?? '';

  const { ...defaults } = getModifierDefaults<Modifier.DateProps>('date', parserOptions);
  const { ...rest } = ownLayer(props, 'date');

  return new Intl.DateTimeFormat(locale, formatOptions(mergeLayer(defaults, rest))).format(input);
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
  const { value, locale = '', props, parserOptions } = config;

  if (!locale) return '';

  const input = getModifierInput(value);

  if (input === undefined) return config.defaultValue ?? '';

  const { format: formatDefault, numeric: numericDefault, ...defaults } = getModifierDefaults<Modifier.AgoProps>('ago', parserOptions);
  const { format = formatDefault ?? 'auto', numeric = numericDefault ?? 'auto', ...rest } = ownLayer(props, 'ago');

  const formatParams = agoFormat(input, format);

  return new Intl.RelativeTimeFormat(locale, formatOptions(mergeLayer(defaults, rest), { numeric })).format(...formatParams);
};

export const currency: Modifier.T<Modifier.CurrencyProps> = (config) => {
  const { value, locale = '', props, parserOptions } = config;

  if (!locale) return '';

  const amount = getModifierInput(value);

  if (amount === undefined) return config.defaultValue ?? '';

  const { ratio: ratioDefault, currency: currencyDefault, ...defaults } = getModifierDefaults<Modifier.CurrencyProps>('currency', parserOptions);
  const { ratio = ratioDefault ?? 1, currency = currencyDefault, ...rest } = ownLayer(props, 'currency');

  const input = getModifierInput(amount * ratio);

  if (input === undefined) return config.defaultValue ?? '';

  // The currency style is what this modifier is, not one of the options it
  // layers: a layer naming another style asks it to stop being the modifier the
  // message named. Pinning it against the parser's defaults alone left the call
  // and the wrapper able to render `{{v:currency}}` as a percentage.
  return new Intl.NumberFormat(locale, formatOptions(mergeLayer(defaults, rest), { style: 'currency', currency })).format(input);
};
