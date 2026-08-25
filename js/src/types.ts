import * as modifiers from './modifiers';

/**
 * A locale as it reaches resolution — an opaque identifier the modifiers hand
 * to `Intl`. The format neither parses nor validates it.
 */
export type Locale = string;

export type CommonProps<CustomModifierProps = Modifier.DefaultProps> = { value: any, props?: CustomModifierProps, locale?: Locale, parserOptions?: Parser.Options };

export type Interpolate = (config: CommonProps & { payload?: Parser.Payload }) => string;

/**
 * The interpolation loop. It sees the message's key on top of what a single
 * pass needs, because a report names the message it came from.
 */
export type Interpolation = (config: Parameters<Interpolate>[0] & { key?: Parser.Key }) => string;

/**
 * A diagnostic the parser hands to its caller. The format does not specify a
 * channel to report through, so the parser writes nowhere itself and describes
 * what happened instead.
 */
export type Report = {
  /** What stopped resolution. */
  code: 'pass-limit' | 'output-limit';
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
   * The text that never settled. Truncated, and with its line terminators
   * escaped, so payload content cannot forge a line wherever this is written.
   */
  text: string;
};

export module Modifier {
  export type Key = string;

  export type DefaultKeys = keyof typeof modifiers;

  export type AgoProps = { ago?: Intl.RelativeTimeFormatOptions & { format?: Intl.RelativeTimeFormatUnit | 'auto' } };

  export type DateProps = { date?: Intl.DateTimeFormatOptions };

  export type NumberProps = { number?: Intl.NumberFormatOptions };

  export type CurrencyProps = { currency?: Intl.NumberFormatOptions & { ratio?: number } };

  export type DefaultProps = NumberProps & AgoProps & DateProps & CurrencyProps;

  export type Props<T = DefaultProps> = T & DefaultProps;

  export type ModifierOption = Record<'key' | 'value', string>;

  export type DefaultValue = string | undefined;

  export type T<CustomModifierProps = any> = (config: CommonProps<CustomModifierProps> & { options: ModifierOption[]; defaultValue?: DefaultValue }) => string;

  export type DefaultModifiers = typeof modifiers;

  export type CustomModifiers<K extends string = any, ModifierProps = any> = Record<K, Modifier.T<ModifierProps>>;
}

export module Parser {
  export type OnReport = (report: Report) => void;

  export type Options<Key extends string = Modifier.Key, Props = any> = {
    customModifiers?: Modifier.CustomModifiers<Key, Props>;
    modifierDefaults?: Modifier.DefaultProps;
    /**
     * Where diagnostics go. Unset, the parser reports nowhere — resolution
     * still fails soft, it just does so silently.
     */
    onReport?: OnReport;
  } | undefined;

  export type PayloadDefault = { [key in 'default']?: any };

  export type Payload<T = any> = [Exclude<keyof T, keyof PayloadDefault>] extends [never] ? Record<string, any> & PayloadDefault : T & PayloadDefault;

  export type Key = string;

  export type Value = any;

  /**
   * Everything resolution reads besides the message itself. `key` is the
   * message's own identifier where the caller has one — it is what a missing
   * message resolves to.
   */
  export type Context<P = PayloadDefault, M = Modifier.DefaultProps> = {
    payload?: Payload<P>;
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
