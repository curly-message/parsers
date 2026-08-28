import type { Modifier } from './types';
import { getDateInput, getModifierDefaults, getModifierInput, mergeLayer, ownLayer } from './utils';

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

  const { maximumFractionDigits: maximumFractionDigitsDefault, ...defaults } = getModifierDefaults<Modifier.NumberProps>('number', parserOptions);
  const { maximumFractionDigits = maximumFractionDigitsDefault ?? 2, ...rest } = ownLayer(props, 'number');

  return new Intl.NumberFormat(locale, { ...mergeLayer(defaults, rest), maximumFractionDigits }).format(input);
};

export const date: Modifier.T<Modifier.DateProps> = ({ value, props, defaultValue = '', locale = '', parserOptions }) => {
  if (!locale) return '';

  const input = getDateInput(value);

  if (input === undefined) return defaultValue;

  const { ...defaults } = getModifierDefaults<Modifier.DateProps>('date', parserOptions);
  const { ...rest } = ownLayer(props, 'date');

  return new Intl.DateTimeFormat(locale, mergeLayer(defaults, rest)).format(input);
};

// The ladder `ago` climbs, each step a multiple of the one below it. A unit
// `Intl` knows but this ladder does not climb can never be selected — the climb
// simply runs out at `year` — so the ladder is also what `Modifier.AgoProps`
// accepts as a format, read straight off this list.
export const agoMap = [
  { key: 'second', multiplier: 1000 },
  { key: 'minute', multiplier: 60 },
  { key: 'hour', multiplier: 60 },
  { key: 'day', multiplier: 24 },
  { key: 'week', multiplier: 7 },
  { key: 'month', multiplier: 13 / 3 },
  { key: 'year', multiplier: 12 },
] as const satisfies readonly { key: Intl.RelativeTimeFormatUnit, multiplier: number }[];

const testResolution = (defKey: string = '', testKey: string = '') => new RegExp(`^${defKey}s?$`).test(testKey);

const findIndex = (currentKey: string) => agoMap.indexOf(agoMap.find(({ key }) => testResolution(key, currentKey)) as any);

const agoFormat = (millis: number, resolution?: Intl.RelativeTimeFormatUnit | 'auto'): [number, Intl.RelativeTimeFormatUnit] => agoMap.reduce(([value, currentKey], { key, multiplier }, index) => {
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
