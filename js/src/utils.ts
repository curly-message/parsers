import type { Parser } from './types';

export const ownValue = (target: any, key?: PropertyKey) => {
  try {
    return key !== undefined && !!target && Object.prototype.hasOwnProperty.call(target, key) ? target[key] : undefined;
  } catch {
    return undefined;
  }
};

export const ownKeys = (target: any) => {
  try {
    return Object.keys(target);
  } catch {
    return [];
  }
};

export const mergeLayer = (base: any, override: any, merge?: (from: any, to: any) => any) => {
  // A null prototype takes `__proto__` as an own key, and the spread that
  // finishes the object defines rather than assigns, so a supplied name stays
  // an own property instead of reaching a prototype.
  const output: Record<string, any> = Object.create(null);

  ownKeys(base).forEach((name) => { output[name] = ownValue(base, name); });

  ownKeys(override).forEach((name) => {
    const to = ownValue(override, name);

    // A name the override sets to `undefined` names nothing, like one it omits.
    if (to === undefined) return;

    output[name] = merge ? merge(ownValue(base, name), to) : to;
  });

  return { ...output };
};

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
