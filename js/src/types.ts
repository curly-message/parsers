import * as modifiers from './modifiers';

/**
 * A locale as it reaches resolution — an opaque identifier the modifiers hand
 * to `Intl`. The format neither parses nor validates it.
 */
export type Locale = string;

export type CommonProps<CustomModifierProps = Modifier.DefaultProps> = { value: any, props?: CustomModifierProps, locale?: Locale, parserOptions?: Parser.Options };

export type Interpolate = (config: CommonProps & { payload?: Parser.Payload }) => string;

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
  export type Options<Key extends string = Modifier.Key, Props = any> = {
    customModifiers?: Modifier.CustomModifiers<Key, Props>;
    modifierDefaults?: Modifier.DefaultProps;
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
