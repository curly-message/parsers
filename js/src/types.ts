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

/**
 * The branches of an all-optional bag that owns at least one of its keys: one
 * per key, each requiring that key be present and leaving the rest optional.
 * A key holding `undefined` is owned; the empty object owns none.
 */
type AtLeastOne<T> = { [Key in keyof T]-?: Record<Key, T[Key]> & Omit<T, Key> }[keyof T];

export type CommonProps<CustomModifierProps = Modifier.DefaultProps> = { value: any, props?: CustomModifierProps, locale?: Locale, parserOptions?: Parser.Options<Modifier.Key, CustomModifierProps> };

/**
 * The text every value a resolution has converted came out as, by identity, the
 * answer that a value has no text included. Converting is the costly step, and
 * a value is read once for every placeholder that names it, so one resolution
 * converts one value once. A primitive is not recorded: its conversion runs no
 * host code and cannot answer twice over, so converting it again is neither
 * observable nor worth an entry.
 */
export type Conversions = Map<any, string | undefined>;

/**
 * A single interpolation pass. It sees the message's key on top of what a pass
 * needs to substitute, because a report names the message it came from, and the
 * conversions the resolution around it has already made. A pass carries the
 * host's props without reading one, so it names no props type of its own.
 */
export type Interpolate = (config: CommonProps<any> & { payload?: Parser.Payload, key?: Parser.Key, conversions: Conversions }) => string;

/** The interpolation loop. */
export type Interpolation = Interpolate;

/**
 * A diagnostic the parser hands to its caller. The format does not specify a
 * channel to report through, so the parser writes nowhere itself and describes
 * what happened instead.
 */
export type Report = {
  /** What stopped resolution. */
  code: 'unknown-modifier' | 'failed-modifier' | 'missing-options' | 'unserializable-value' | 'missing-locale' | 'pass-limit' | 'output-limit';
  /**
   * Which of the three the defect belongs to, and so who fixes it: the message
   * that was written, what the caller passed, or a limit this parser set. Every
   * code declares one. It ranks nothing — a report is no graver for coming from
   * one of the three than from another.
   */
  origin: 'message' | 'payload' | 'limit';
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
   * Where the trouble came from: the placeholder for a report about one, the
   * output that would not settle for the two limits, and nothing at all where
   * what could not be described is the chain a message itself resolves
   * through, which names no placeholder. Truncated — a cut is marked with a
   * trailing `...` of the parser's own — and with its line terminators
   * escaped, so payload content cannot forge a line wherever this is written.
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
   * step is accepted in the plural too, so a layer naming one may spell it
   * either way.
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
   * would. An entry is a wrapper only when it is a plain object that owns at
   * least one key and every key it owns is one of these; an entry with a
   * prototype of its own, one owning anything else, or one owning nothing at
   * all, is a value, wrapper-shaped or not.
   */
  export type Wrapper<Value = any, CustomModifierProps = DefaultProps> = AtLeastOne<{
    /** The value itself. A wrapper carrying none falls back like a missing key. */
    value?: Value;
    /** Tried before the payload's own `default` and before the inline one. */
    default?: DefaultValue;
    /** Layered over the `props` the call passes, property by property. */
    props?: Props<CustomModifierProps>;
  }>;

  export type T<CustomModifierProps = DefaultProps> = (config: CommonProps<CustomModifierProps> & {
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

  export type CustomModifiers<K extends string = any, ModifierProps = DefaultProps> = Record<K, Modifier.T<ModifierProps>>;
}

export module Parser {
  export type OnReport = (report: Report) => void;

  export type Options<Key extends string = Modifier.Key, Props = Modifier.DefaultProps> = {
    /**
     * Modifiers registered by name, over the built-in ones. A name a message
     * may write is one the parser holds a modifier under or one a host
     * registered one under, so an entry that is not a modifier registers none:
     * it takes no name of its own and shadows no built-in. The name then reads
     * as one nobody registered where nothing else answers to it, and answers
     * as it did where a built-in does.
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
  };

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
   * `lt` and `gt` over a `Date` resolve to the fallback chain — the three
   * formatting ones given a locale, because with none they resolve to the
   * empty string whatever the value is. A timestamp or an ISO string keeps
   * both.
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
