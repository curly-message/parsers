import type { Modifier } from './types';
import { AGO_LADDER, getDateInput, getModifierDefaults, getModifierInput, mergeLayer, ownLayer, ownValue } from './utils';

// A selection that matched answers with its own value, empty or not. Only a
// selection that matched nothing falls back.
const selected = (option: Modifier.ModifierOption | undefined, defaultValue: any) => (option ? option.value : defaultValue);

export const eq: Modifier.T = ({ value, options = [], defaultValue = '' }) => selected(options.find(
  ({ key }) => `${key}`.toLowerCase() === `${value}`.toLowerCase(),
), defaultValue);

export const ne: Modifier.T = ({ value, options = [], defaultValue = '' }) => selected(options.find(
  ({ key }) => `${key}`.toLowerCase() !== `${value}`.toLowerCase(),
), defaultValue);

// A numeric comparison reads only the options it can order. A key that is not
// numeric can never be selected by one, and leaving it in the list would leave
// the comparator answering NaN, which sorts as equal and freezes the pairs
// around it. Ordering is done on a copy, so the caller's list keeps its order
// for the `eq` leg `lte` and `gte` run over it.
const ordered = (options: Modifier.ModifierOption[], compare: (a: number, b: number) => number) => options
  .filter(({ key }) => !Number.isNaN(+key))
  .sort((a, b) => compare(+a.key, +b.key));

export const lt: Modifier.T = ({ value, options = [], defaultValue = '' }) => selected(ordered(options, (a, b) => a - b).find(
  ({ key }) => +value < +key,
), defaultValue);

export const gt: Modifier.T = ({ value, options = [], defaultValue = '' }) => selected(ordered(options, (a, b) => b - a).find(
  ({ key }) => +value > +key,
), defaultValue);

export const lte: Modifier.T = ({ value, options = [], defaultValue = '' }) => eq({ value, options, defaultValue: lt({ value, options, defaultValue }) });

export const gte: Modifier.T = ({ value, options = [], defaultValue = '' }) => eq({ value, options, defaultValue: gt({ value, options, defaultValue }) });

export const number: Modifier.T<Modifier.NumberProps> = ({ value, props, defaultValue = '', locale = '', parserOptions }) => {
  if (!locale) return '';

  const input = getModifierInput(value);

  if (input === undefined) return defaultValue;

  const layered = mergeLayer(getModifierDefaults<Modifier.NumberProps>('number', parserOptions), ownLayer(props, 'number'));
  // Two fraction digits is what this modifier formats when nobody named a
  // maximum: a default, not a cap. `Intl` widens its own default maximum to
  // reach a larger minimum, and this default widens the same way — held at two
  // over a layer's `minimumFractionDigits`, it would contradict it, `Intl`
  // would raise, and the number would resolve to a fallback nobody asked for.
  const minimum = Number(ownValue(layered, 'minimumFractionDigits')) || 0;
  const maximumFractionDigits = ownValue(layered, 'maximumFractionDigits') ?? Math.max(minimum, 2);

  return new Intl.NumberFormat(locale, { ...layered, maximumFractionDigits }).format(input);
};

export const date: Modifier.T<Modifier.DateProps> = ({ value, props, defaultValue = '', locale = '', parserOptions }) => {
  if (!locale) return '';

  const input = getDateInput(value);

  if (input === undefined) return defaultValue;

  const { ...defaults } = getModifierDefaults<Modifier.DateProps>('date', parserOptions);
  const { ...rest } = ownLayer(props, 'date');

  return new Intl.DateTimeFormat(locale, mergeLayer(defaults, rest)).format(input);
};

const testResolution = (defKey: string = '', testKey: string = '') => new RegExp(`^${defKey}s?$`).test(testKey);

const findIndex = (currentKey: string) => AGO_LADDER.indexOf(AGO_LADDER.find(({ key }) => testResolution(key, currentKey)) as any);

const agoFormat = (millis: number, resolution?: Intl.RelativeTimeFormatUnit | 'auto'): [number, Intl.RelativeTimeFormatUnit] => AGO_LADDER.reduce(([value, currentKey], { key, multiplier }, index) => {
  if (testResolution(currentKey, resolution)) return [value, currentKey];

  if (!currentKey || index === findIndex(currentKey) + 1) {
    const output = Math.round(value / multiplier);

    if (!currentKey || Math.abs(output) >= 1 || resolution !== 'auto') return [output, key];
  }

  return [value, currentKey];
}, [millis, '' as Intl.RelativeTimeFormatUnit]);

export const ago: Modifier.T<Modifier.AgoProps> = ({ value, defaultValue = '', locale = '', props, parserOptions }) => {
  if (!locale) return '';

  const input = getModifierInput(value);

  if (input === undefined) return defaultValue;

  const { format: formatDefault, numeric: numericDefault, ...defaults } = getModifierDefaults<Modifier.AgoProps>('ago', parserOptions);
  const { format = formatDefault ?? 'auto', numeric = numericDefault ?? 'auto', ...rest } = ownLayer(props, 'ago');

  const formatParams = agoFormat(input, format);

  return new Intl.RelativeTimeFormat(locale, { ...mergeLayer(defaults, rest), numeric }).format(...formatParams);
};

export const currency: Modifier.T<Modifier.CurrencyProps> = ({ value, defaultValue = '', locale = '', props, parserOptions }) => {
  if (!locale) return '';

  const amount = getModifierInput(value);

  if (amount === undefined) return defaultValue;

  const { ratio: ratioDefault, currency: currencyDefault, ...defaults } = getModifierDefaults<Modifier.CurrencyProps>('currency', parserOptions);
  const { ratio = ratioDefault ?? 1, currency = currencyDefault, ...rest } = ownLayer(props, 'currency');

  const input = getModifierInput(amount * ratio);

  if (input === undefined) return defaultValue;

  return new Intl.NumberFormat(locale, { ...mergeLayer({ ...defaults, style: 'currency' }, rest), currency }).format(input);
};
