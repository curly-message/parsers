import * as modifiers from './modifiers';
import type { AGO_LADDER } from './utils';

/**
 * A locale as it reaches resolution — an opaque identifier the modifiers hand
 * to `Intl`. The format neither parses nor validates it.
 */
export type Locale = string;

/**
 * Holds a position out of the inference the surrounding call does. A type
 * parameter is decided where it is declared or where its own argument names it
 * — never by a second position that merely has to agree with it.
 */
type Given<T> = [T][T extends any ? 0 : never];

export type CommonProps<CustomModifierProps = Modifier.DefaultProps> = { value: any, props?: CustomModifierProps, locale?: Locale, parserOptions?: Parser.Options };

/**
 * The text every value a resolution has converted came out as, by identity, the
 * answer that a value has no text included. Converting is the costly step, and
 * a value is read once for every placeholder that names it, so one resolution
 * converts one value once.
 */
export type Conversions = Map<any, string | undefined>;

/**
 * A single interpolation pass. It sees the message's key on top of what a pass
 * needs to substitute, because a report names the message it came from, and the
 * conversions the resolution around it has already made.
 */
export type Interpolate = (config: CommonProps & { payload?: Parser.Payload, key?: Parser.Key, conversions: Conversions }) => string;

/** The interpolation loop. */
export type Interpolation = Interpolate;

/**
 * A diagnostic the parser hands to its caller. The format does not specify a
 * channel to report through, so the parser writes nowhere itself and describes
 * what happened instead.
 */
export type Report = {
  /** What stopped resolution. */
  code: 'unknown-modifier' | 'failed-modifier' | 'unserializable-value' | 'pass-limit' | 'output-limit';
  /**
   * The same in English. It is self-contained and carries nothing from the
   * payload, so writing it anywhere is safe without further thought.
   */
  message: string;
  /** The message's own key, where the caller passed one. */
  key?: Parser.Key;
  /** The limit that was reached, where the report is about one. */
  limit?: number;
  /**
   * The text that never settled. Truncated — a cut is marked with a trailing
   * `...` of the parser's own — and with its line terminators escaped, so
   * payload content cannot forge a line wherever this is written.
   */
  text: string;
};

export module Modifier {
  export type Key = string;

  export type DefaultKeys = keyof typeof modifiers;

  type AgoStep = (typeof AGO_LADDER)[number]['key'];

  /**
   * A unit `ago` can resolve to, read off the ladder it climbs rather than off
   * `Intl`'s whole vocabulary: a unit the ladder does not climb is never
   * selected, it just leaves the climb running out at its largest step. Every
   * step is accepted in the plural too, which is how a message spells it.
   */
  export type AgoUnit = AgoStep | `${AgoStep}s`;

  export type AgoProps = { ago?: Intl.RelativeTimeFormatOptions & { format?: AgoUnit | 'auto' } };

  export type DateProps = { date?: Intl.DateTimeFormatOptions };

  export type NumberProps = { number?: Intl.NumberFormatOptions };

  export type CurrencyProps = { currency?: Intl.NumberFormatOptions & { ratio?: number } };

  export type DefaultProps = NumberProps & AgoProps & DateProps & CurrencyProps;

  export type Props<T = DefaultProps> = T & DefaultProps;

  export type ModifierOption = Record<'key' | 'value', string>;

  /**
   * What a placeholder falls back to. It reaches a modifier as text whether
   * the message declared it or the payload carried it — a payload default is
   * coerced exactly like a value.
   */
  export type DefaultValue = any;

  /**
   * A value's own configuration, standing in the payload where the value
   * would. An entry is a wrapper only when it owns at least one key and every
   * key it owns is one of these; an entry owning anything else, or owning
   * nothing at all, is a value, wrapper-shaped or not.
   */
  export type Wrapper<Value = any, CustomModifierProps = DefaultProps> = {
    /** The value itself. A wrapper carrying none falls back like a missing key. */
    value?: Value;
    /** Tried before the payload's own `default` and before the inline one. */
    default?: DefaultValue;
    /** Layered over the `props` the call passes, property by property. */
    props?: Props<CustomModifierProps>;
  };

  export type T<CustomModifierProps = any> = (config: CommonProps<CustomModifierProps> & {
    options: ModifierOption[];
    /**
     * The fallback chain, resolved by the read rather than before the modifier
     * was called: reading it walks the wrapper's `default`, then the payload's,
     * then the one the placeholder declared, runs whatever host code those
     * links carry, and reports one no conversion describes. A generic copy of
     * the config is such a read — a rest destructure, a spread,
     * `JSON.stringify` — so a modifier with no use for the default takes the
     * keys it needs by name.
     */
    defaultValue?: DefaultValue;
  }) => any;

  export type DefaultModifiers = typeof modifiers;

  export type CustomModifiers<K extends string = any, ModifierProps = any> = Record<K, Modifier.T<ModifierProps>>;
}

export module Parser {
  export type OnReport = (report: Report) => void;

  export type Options<Key extends string = Modifier.Key, Props = any> = {
    /**
     * Modifiers registered by name, over the built-in ones. Registration is
     * what makes a name one a message may write, so an entry that is not a
     * modifier registers none: it takes no name of its own and shadows no
     * built-in, and the name reads as one nobody registered.
     */
    customModifiers?: Modifier.CustomModifiers<Key, Props>;
    /**
     * The bottom formatting layer, keyed by modifier name. It carries the same
     * names the call's own `props` does, host-defined modifiers included — a
     * modifier a host can configure per call it can also give defaults.
     */
    modifierDefaults?: Modifier.Props<Given<Props>>;
    /**
     * Where diagnostics go. Unset, the parser reports nowhere — resolution
     * still fails soft, it just does so silently.
     */
    onReport?: OnReport;
  } | undefined;

  export type PayloadDefault = { [key in 'default']?: any };

  /** What a payload carries under a key: the value, or its configuration. */
  export type PayloadEntry<Value = any, Props = Modifier.DefaultProps> = Value | Modifier.Wrapper<Value, Props>;

  /**
   * The values a message's placeholders name, plus `default` — the fallback
   * for every key the payload does not carry.
   *
   * A value reaches a modifier as text: a plain object and an array become
   * JSON, and anything else becomes what the host makes of it. An entry may
   * instead be a `Modifier.Wrapper`, which configures the value it carries.
   *
   * A `Date` loses its sub-second precision to that conversion, and the text
   * `String` writes for one is not numeric, so `number`, `currency`, `ago`,
   * `lt` and `gt` over a `Date` resolve to the fallback chain. A timestamp or
   * an ISO string keeps both.
   *
   * A value passes through the same unescaping as the message around it: a
   * backslash before a character the syntax reserves — `:`, `;`, `{`, `}`, a
   * backslash, or whitespace — writes that character as text and is dropped
   * itself, while a backslash before anything else is left as it is. So a
   * value holding `\d+` arrives as typed, and one holding `\\server\share`
   * resolves to `\server\share` unless each consumed backslash is doubled.
   */
  export type Payload<T = any, Props = Modifier.DefaultProps> = [Exclude<keyof T, keyof PayloadDefault>] extends [never] ? Record<string, PayloadEntry<any, Props>> & PayloadDefault : { [Key in keyof T]: PayloadEntry<T[Key], Props> } & PayloadDefault;

  export type Key = string;

  export type Value = any;

  /**
   * Everything resolution reads besides the message itself. `key` is the
   * message's own identifier where the caller has one. A missing message
   * resolves to the payload's own `default`, and to `key` where the payload
   * carries none — the same chain a placeholder falls through, one level up.
   */
  export type Context<P = PayloadDefault, M = Modifier.DefaultProps> = {
    payload?: Payload<P, M>;
    props?: Modifier.Props<M>;
    locale?: Locale;
    key?: Key;
  };

  export type Resolve<C extends Parser.Context = Parser.Context> = (message: Value, context?: C) => string;

  export type T<C extends Parser.Context = Parser.Context> = {
    /**
     * Interpolates the message against the given context and returns the result.
     */
    resolve: Resolve<C>;
  };

  /**
   * The payload type comes first: with no host config to carry it, the factory
   * is where a caller declares what its messages expect.
   */
  export type Factory = <Payload = {}, Props = {}, Key extends string = Modifier.Key>(options?: Parser.Options<Key, Props>) => Parser.T<Parser.Context<Payload & PayloadDefault, Props & Modifier.DefaultProps>>;
}
