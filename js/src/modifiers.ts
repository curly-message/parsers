import type { Modifier } from './types';
import { getModifierDefaults, getModifierInput, mergeLayer } from './utils';

// A selection that matched answers with its own value, empty or not. Only a
// selection that matched nothing falls back.
const selected = (option: Modifier.ModifierOption | undefined, defaultValue: any) => (option ? option.value : defaultValue);

export const eq: Modifier.T = ({ value, options = [], defaultValue = '' }) => selected(options.find(
  ({ key }) => `${key}`.toLowerCase() === `${value}`.toLowerCase(),
), defaultValue);

export const ne: Modifier.T = ({ value, options = [], defaultValue = '' }) => selected(options.find(
  ({ key }) => `${key}`.toLowerCase() !== `${value}`.toLowerCase(),
), defaultValue);

export const lt: Modifier.T = ({ value, options = [], defaultValue = '' }) => {
  const sortedOptions = options.sort((a, b) => +a.key - +b.key);

  return selected(sortedOptions.find(
    ({ key }) => +value < +key,
  ), defaultValue);
};

export const gt: Modifier.T = ({ value, options = [], defaultValue = '' }) => {
  const sortedOptions = options.sort((a, b) => +b.key - +a.key);

  return selected(sortedOptions.find(
    ({ key }) => +value > +key,
  ), defaultValue);
};

export const lte: Modifier.T = ({ value, options = [], defaultValue = '' }) => eq({ value, options, defaultValue: lt({ value, options, defaultValue }) });

export const gte: Modifier.T = ({ value, options = [], defaultValue = '' }) => eq({ value, options, defaultValue: gt({ value, options, defaultValue }) });

export const number: Modifier.T<Modifier.NumberProps> = ({ value, props, defaultValue = '', locale = '', parserOptions }) => {
  if (!locale) return '';

  const input = getModifierInput(value);

  if (input === undefined) return defaultValue;

  const { maximumFractionDigits: maximumFractionDigitsDefault, ...defaults } = getModifierDefaults<Modifier.NumberProps>('number', parserOptions);
  const { maximumFractionDigits = maximumFractionDigitsDefault || 2, ...rest } = props?.number || {};

  return new Intl.NumberFormat(locale, { ...mergeLayer(defaults, rest), maximumFractionDigits }).format(input);
};

export const date: Modifier.T<Modifier.DateProps> = ({ value, props, defaultValue = '', locale = '', parserOptions }) => {
  if (!locale) return '';

  const input = getModifierInput(value);

  if (input === undefined) return defaultValue;

  const { ...defaults } = getModifierDefaults<Modifier.DateProps>('date', parserOptions);
  const { ...rest } = props?.date || {};

  return new Intl.DateTimeFormat(locale, mergeLayer(defaults, rest)).format(input);
};

const agoMap = [
  { key: 'second', multiplier: 1000 },
  { key: 'minute', multiplier: 60 },
  { key: 'hour', multiplier: 60 },
  { key: 'day', multiplier: 24 },
  { key: 'week', multiplier: 7 },
  { key: 'month', multiplier: 13 / 3 },
  { key: 'year', multiplier: 12 },
] as { key: Intl.RelativeTimeFormatUnit, multiplier: number }[];

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
  const { format = formatDefault || 'auto', numeric = numericDefault || 'auto', ...rest } = props?.ago || {};

  const formatParams = agoFormat(input, format);

  return new Intl.RelativeTimeFormat(locale, { ...mergeLayer(defaults, rest), numeric }).format(...formatParams);
};

export const currency: Modifier.T<Modifier.CurrencyProps> = ({ value, defaultValue = '', locale = '', props, parserOptions }) => {
  if (!locale) return '';

  const { ratio: ratioDefault, currency: currencyDefault, ...defaults } = getModifierDefaults<Modifier.CurrencyProps>('currency', parserOptions);
  const { ratio = ratioDefault || 1, currency = currencyDefault, ...rest } = props?.currency || {};

  const input = getModifierInput(value * ratio);

  if (input === undefined) return defaultValue;

  return new Intl.NumberFormat(locale, { ...mergeLayer({ ...defaults, style: 'currency' }, rest), currency }).format(input);
};
