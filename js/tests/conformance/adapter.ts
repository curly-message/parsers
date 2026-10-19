import type { Adapter, ModifierInput } from '@curly-message/conformance';
import { createParser, cst, Modifier, Parser, Report } from '@curly-message/parser';

type Input = Parameters<Adapter['resolve']>[0];

/**
 * A parser configured as a case of the set asks, reporting into `reports`.
 * A behaviour of the set's catalogue is a function of section 11's inputs;
 * this parser's modifier signature carries the same inputs under its own
 * names, with the default behind an accessor that walks the chain on the
 * read, which is what the behaviour's `default` call is for.
 */
export const parserFor = ({ modifiers, defaults }: Pick<Input, 'modifiers' | 'defaults'>, reports: Report[]) => {
  const wrap = (behaviour: (input: ModifierInput) => unknown): Modifier.T => (config) => behaviour({
    value: config.value,
    options: config.options,
    props: config.props,
    locale: config.locale,
    default: () => config.defaultValue,
  });

  return createParser({
    customModifiers: modifiers && Object.fromEntries(Object.entries(modifiers).map(([name, behaviour]) => [name, wrap(behaviour)])),
    modifierDefaults: defaults as Modifier.Props | undefined,
    onReport: (report) => { reports.push(report); },
  });
};

/** The second argument of `resolve` a case of the set asks for. */
export const contextOf = ({ payload, props, locale, id }: Input): Parser.Context => ({ payload: payload as Parser.Payload | undefined, props: props as Modifier.Props | undefined, locale, id: id as Parser.Id | undefined });

/**
 * The adapter of SPEC.md section 14.3: what this implementation supplies so
 * the conformance set can drive it. The levels and the limits are the two
 * statements the specification asks an implementation to make about itself,
 * and are the ones the README makes; the limits are the constants the parser
 * enforces, spelled here because the set derives its boundary cases from what
 * an implementation documents rather than from section 13's minima.
 */
export const adapter: Adapter = {
  levels: ['core', 'intl', 'extensions'],
  limits: { output: 100000, read: 100000, conversion: 100000, nesting: 8 },
  // CST.md section 4 lets an implementation count its spans in whatever unit
  // its strings are indexed by, and asks it to say which: these are ECMAScript
  // string indices, so UTF-16 code units.
  cst: { unit: 'utf-16', parse: cst },
  resolve: (input) => {
    const reports: Report[] = [];
    const { resolve } = parserFor(input, reports);

    return { output: resolve(input.message, contextOf(input)), reports };
  },
};
