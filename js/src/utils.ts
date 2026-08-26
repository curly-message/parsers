import type { Parser } from './types';

export const getModifierDefaults = <T>(key: keyof T, parserOptions: Parser.Options) => {
  const { modifierDefaults } = parserOptions || {};
  const { [key]: output } = modifierDefaults || {};

  return (output || {}) as Required<T>[keyof T];
};

/**
 * The number a formatting modifier will format, by the host's own conversion.
 * A value that does not convert is one the modifier cannot format — the caller
 * resolves it to the fallback chain instead.
 */
export const getModifierInput = (value: any) => {
  const input = +value;

  return Number.isFinite(input) ? input : undefined;
};
