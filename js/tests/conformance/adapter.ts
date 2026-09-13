import type { Adapter, ModifierInput } from '@curly-message/conformance';
import { createParser, Modifier, Parser, Report } from '../../src';

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
  limits: { passes: 10, output: 100000, conversion: 100000 },
  resolve: ({ message, payload, props, locale, id, modifiers, defaults }) => {
    const reports: Report[] = [];

    // A behaviour of the set's catalogue is a function of section 11's inputs;
    // this parser's modifier signature carries the same inputs under its own
    // names, with the default behind an accessor that walks the chain on the
    // read, which is what the behaviour's `default` call is for.
    const wrap = (behaviour: (input: ModifierInput) => unknown): Modifier.T => (config) => behaviour({
      value: config.value,
      options: config.options,
      props: config.props,
      locale: config.locale,
      default: () => config.defaultValue,
    });

    const { resolve } = createParser({
      customModifiers: modifiers && Object.fromEntries(Object.entries(modifiers).map(([name, behaviour]) => [name, wrap(behaviour)])),
      modifierDefaults: defaults as Modifier.Props | undefined,
      onReport: (report) => { reports.push(report); },
    });

    return {
      output: resolve(message, { payload: payload as Parser.Payload | undefined, props: props as Modifier.Props | undefined, locale, id: id as Parser.Id | undefined }),
      reports,
    };
  },
};
