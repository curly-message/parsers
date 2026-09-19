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

/**
 * The text every value a resolution has converted came out as, keyed by the
 * value the conversion read, the answer that a value has no text included.
 * Converting is the costly step, and a value is read once for every placeholder
 * that names it, so one resolution converts one value once. A primitive whose
 * conversion runs no host code and costs nothing to repeat is not recorded:
 * converting it again is neither observable nor worth an entry. A bigint is,
 * because the digits it converts to are work the engine charges for on every
 * read.
 */
export type Conversions = Map<any, string | undefined>;

/**
 * Whether each entry a resolution has asked configures its value, keyed by the
 * entry, the answer that the entry refused the question included. Recognizing
 * a wrapper enumerates the entry's own names, which reads the entry as a
 * conversion does and costs what the entry holds, and an entry is read once
 * for every placeholder that names it, so one resolution asks one entry once.
 */
export type Wrappers = Map<object, boolean | undefined>;

/**
 * What the walk reads. It sees the message's id on top of what resolving one
 * needs, because a report names the message it came from, and what the call
 * around it has already converted and recognized. The option bag's own entries
 * reach it read: the modifiers a message can name and the defaults they read
 * are the call's own structure, read once for the call like the context's
 * entries, so the walk composes over them and asks the bag nothing. The walk
 * carries the host's props without reading one, so it names no props type of
 * its own.
 */
type WalkProps = { value: any, props?: any, locale?: Locale, parserOptions?: Parser.Options<Modifier.Key, any>, modifiers: Record<string, Modifier.T<any, any>>, modifierDefaults?: Modifier.Props, onReport?: Parser.OnReport, onSuspectValue?: Parser.OnSuspectValue, recognizeWrappers: boolean, payload?: Parser.Payload, id?: Parser.Id, conversions: Conversions, wrappers: Wrappers };

/**
 * Resolves a message. One walk produces the whole output: what a placeholder
 * resolves to is data and is never read back as syntax, so there is nothing to
 * repeat and nothing to answer with but the text.
 */
export type Interpolation = (config: WalkProps) => string;

/**
 * A diagnostic the parser hands to its caller. The format does not specify a
 * channel to report through, so the parser writes nowhere itself and describes
 * what happened instead.
 */
export type Report = {
  /** What stopped resolution. */
  code: 'unknown-modifier' | 'failed-modifier' | 'missing-options' | 'unserializable-value' | 'missing-locale' | 'nesting-limit' | 'output-limit' | 'read-limit';
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
  /** The message's own id, where the caller passed one. */
  id?: Parser.Id;
  /** The limit that was reached, where the report is about one. */
  limit?: number;
  /**
   * Where the trouble came from: the placeholder for a report about one, and
   * the message as the caller wrote it where a read of the call's own
   * structure refused — a context entry, an entry of the option bag. That one
   * names no placeholder, and a message that is not text carries none of
   * itself either. It is message text throughout and never a payload value:
   * what a placeholder resolves to is data, and nothing reads it as text
   * again. Truncated — a cut is marked with a trailing `...` of the parser's
   * own — and with its line terminators escaped, so no message can forge a
   * line wherever this is written.
   */
  text: string;
};

export module Modifier {
  export type Key = string;

  export type DefaultKeys = keyof typeof modifiers;

  type AgoStep = (typeof AGO_LADDER)[number]['key'];

  /**
   * A unit `ago` can resolve to, read off the ladder it climbs rather than off
   * `Intl`'s whole vocabulary: a `format` naming a unit the ladder does not
   * climb is one the modifier cannot format with, so the placeholder takes the
   * fallback chain and the failure is reported. Every step is accepted in the
   * plural too, so a layer naming one may spell it either way.
   */
  export type AgoUnit = AgoStep | `${AgoStep}s`;

  /**
   * What a modifier of this name is handed: the properties composed under it,
   * which is what the layers keyed by that name carry. The `*Props` type
   * beside each is the layer itself — one name, holding those properties.
   */
  export type AgoProperties = Intl.RelativeTimeFormatOptions & { format?: AgoUnit | 'auto' };

  export type AgoProps = { ago?: AgoProperties };

  export type DateProperties = Intl.DateTimeFormatOptions;

  export type DateProps = { date?: DateProperties };

  export type NumberProperties = Intl.NumberFormatOptions;

  export type NumberProps = { number?: NumberProperties };

  export type CurrencyProperties = Intl.NumberFormatOptions & { ratio?: number };

  export type CurrencyProps = { currency?: CurrencyProperties };

  export type DefaultProps = NumberProps & AgoProps & DateProps & CurrencyProps;

  export type Props<T = DefaultProps> = T & DefaultProps;

  export type ModifierOption = Record<'key' | 'value', string>;

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
    default?: any;
    /** Layered over the `props` the call passes, property by property. */
    props?: Props<CustomModifierProps>;
  }>;

  /**
   * A modifier is handed text and nothing else: a placeholder whose value is
   * absent takes its fallback chain before any modifier is called, and that
   * chain ends in the empty string.
   *
   * `OwnProps` is what this modifier's own name holds, because that
   * composition is what it is handed; `CustomModifierProps` is the table that
   * name sits in, which is what `parserOptions` is read under. A modifier
   * registered through `customModifiers` is given both by the table it is
   * registered in, so only one written down away from its table names them.
   * The empty composition is the default, because that is what a name nobody
   * configured holds.
   */
  export type T<OwnProps = {}, CustomModifierProps = DefaultProps> = (config: {
    value: string;
    /**
     * The properties composed under this modifier's own name, layered from the
     * implementation defaults up through the wrapper's own and copied, so a
     * modifier that writes into what it was handed reaches neither the next
     * placeholder nor the caller. A modifier nobody configured is handed an
     * empty object.
     */
    props: OwnProps;
    locale?: Locale;
    parserOptions?: Parser.Options<Modifier.Key, CustomModifierProps>;
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
    defaultValue: string;
  }) => any;

  export type DefaultModifiers = typeof modifiers;

  /**
   * Modifiers by the name each answers to. A name the table declares
   * properties for holds a modifier reading that slice; one it declares none
   * for holds a modifier reading none by name. The slice reaches an entry
   * through the names the table is typed with — the ones the factory infers
   * from the table it is given, or the ones an annotation names — so a table
   * or an option bag annotated without them types every entry, one under a
   * built-in name included, as reading none.
   */
  export type CustomModifiers<K extends string = Key, ModifierProps = DefaultProps> = {
    [Name in K]: Modifier.T<Name extends keyof Props<ModifierProps> ? NonNullable<Props<ModifierProps>[Name]> : {}, ModifierProps>
  };
}

export module Parser {
  export type OnReport = (report: Report) => void;

  /**
   * What a value holds that version 1 of this format would have read as
   * syntax: `placeholder` where it holds `{{`, `escape` where it holds a
   * backslash. A value holding both is described by both, in that order.
   *
   * Neither is anything since version 2 — a value is data and reaches the
   * output as it stands — which is the whole point of that version and the
   * whole reason a catalogue written for version 1 may render differently
   * under it.
   */
  export type SuspectKind = 'placeholder' | 'escape';

  /**
   * A value a placeholder read that version 1 would have read as syntax. It
   * is not a `Report`: nothing went wrong, the placeholder resolved to exactly
   * the text the payload holds, and no code of section 14.3 describes that.
   * It is a migration aid, and a host that is not migrating asks for none.
   */
  export type Suspect = {
    found: readonly SuspectKind[];
    /** The placeholder that read the value, as the message spells it. */
    placeholder: string;
    /** The message's own id, where the caller passed one. */
    id?: Id;
    /**
     * The value's text, truncated and with its line terminators escaped the
     * way a report's text is. Unlike a report's, this **is** payload text: a
     * host that writes it somewhere writes what its payload holds.
     */
    text: string;
  };

  export type OnSuspectValue = (suspect: Suspect) => void;

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
     * Where diagnostics go. Unset or `null`, the parser reports nowhere —
     * resolution still fails soft, it just does so silently. `null` is for a
     * host that states the silence rather than omits it.
     */
    onReport?: OnReport | null;
    /**
     * Whether a payload entry shaped like a `Modifier.Wrapper` configures its
     * value (section 4.1). On where the caller says nothing.
     *
     * `false` turns it off: an entry of that shape is then a value like any
     * other and converts as one, so it carries no `props` and no `default`.
     * That is where a caller holding untrusted data passes it — an entry it
     * did not write, shaped like a wrapper, otherwise reconfigures every
     * modifier the placeholder reaches without spelling any syntax
     * (section 14.1).
     */
    recognizeWrappers?: boolean;
    /**
     * Where a value that version 1 of this format would have read as syntax is
     * announced. Unset or `null`, nothing is announced and nothing is looked
     * for.
     *
     * This is a migration aid, on a channel of its own because it is not a
     * report: the placeholder resolved correctly, to the text the payload
     * holds. A catalogue written for version 1 that composed messages through
     * its payload renders that composition literally now, and this says which
     * values those are while a host is looking for them. Turn it off once the
     * catalogue is migrated — every value read is searched while it is on.
     */
    onSuspectValue?: OnSuspectValue | null;
  };

  export type PayloadDefault = { [key in 'default']?: any };

  /** What a payload carries under a key: the value, or its configuration. */
  export type PayloadEntry<Value = any, Props = Modifier.DefaultProps> = Value | Modifier.Wrapper<Value, Props>;

  /**
   * The values a message's placeholders name, plus `default` — the fallback
   * for every key the payload does not carry.
   *
   * A value reaches a modifier as text: a plain object and a plain array become
   * JSON, and anything else becomes what the host makes of it — a class
   * instance, and an array of a type derived from `Array` or built in another
   * realm, among them. An entry may instead be a `Modifier.Wrapper`, which
   * configures the value it carries.
   *
   * A `Date` loses its sub-second precision to that conversion, and the text
   * `String` writes for one is not numeric, so `number`, `currency`, `ago`,
   * `lt` and `gt` over a `Date` resolve to the fallback chain — the three
   * formatting ones given a locale, because with none they resolve to the
   * empty string whatever the value is. A timestamp or an ISO string keeps
   * both.
   *
   * A value is data and is never read as syntax: no escape sequence is removed
   * from it and no placeholder is found in it, so it reaches the output as it
   * was passed. A value holding `\d+` renders `\d+`, one holding `\\server\share`
   * renders `\\server\share`, and one holding the nine characters `{{count}}`
   * renders those nine characters.
   */
  export type Payload<T = any, Props = Modifier.DefaultProps> = [Exclude<keyof T, keyof PayloadDefault>] extends [never] ? Record<string, PayloadEntry<any, Props>> & PayloadDefault : { [Key in keyof T]: PayloadEntry<T[Key], Props> } & PayloadDefault;

  export type Id = string;

  export type Value = any;

  /**
   * Everything a resolution is given besides the message itself. `id` is the
   * message's own identifier where the caller has one; no step of resolution
   * reads it, and a report names it so that it says which message went
   * looking.
   */
  export type Context<P = PayloadDefault, M = Modifier.DefaultProps> = {
    payload?: Payload<P, M>;
    props?: Modifier.Props<M>;
    locale?: Locale;
    id?: Id;
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

  /**
   * What a parameter accepts, as far as the message says. `'unknown'` is the
   * top of this lattice rather than a conflict marker: a parameter every value
   * satisfies is one the message narrows not at all, and merging it with any
   * other kind leaves the other. `'date'` is a timestamp or a date instance;
   * `'string'` accompanies it wherever text the host reads as a date is
   * accepted too, and accompanies `'number'` wherever an exact text match
   * selects beside a numeric order.
   */
  export type ParamKind = 'unknown' | 'string' | 'number' | 'date';

  /** One parameter a message names. */
  export type ParamSpec = {
    /**
     * The payload key the placeholder names, already unescaped. A key is
     * arbitrary text rather than an identifier, so whatever writes it down
     * quotes it.
     */
    name: string;
    /**
     * What the parameter accepts, taking every placeholder naming it
     * together. Several kinds mean the message reads the parameter in several
     * ways and any of them is valid; `unknown` is the top of that lattice
     * rather than a member of it, so it is reported alone and only while
     * nothing has narrowed the parameter.
     */
    kind: ParamKind | readonly ParamKind[];
    /**
     * The values the message names explicitly: the option keys of an `eq`
     * selection, which is the one comparison whose keys are values of the
     * parameter rather than thresholds it is ordered against or a value it
     * must differ from. A hint and never a closed set — a value none of them
     * matches resolves through the fallback chain rather than failing — and
     * absent where the message names none.
     */
    values?: readonly string[];
    /**
     * Whether the message states a fallback for the parameter. Every
     * placeholder renders without its value, so this reports what the message
     * says rather than what resolution tolerates: a placeholder declaring an
     * inline `default` says the value may be missing, and one declaring none
     * says it is expected.
     */
    optional: boolean;
  };

  /**
   * The parameters a message names, in the order it first names each. This is
   * the build-time half of the parser: a message scanner is of no use at
   * render time, so resolution never calls it and a bundle that never reaches
   * it drops it.
   *
   A message that is not text names no parameters rather than raising: a
   * catalogue leaf is arbitrary data. Text is also all that is scanned, and a
   * payload is never read as text, so a placeholder a value carries is not one
   * the message names.
   */
  export type Extractor = (message: Value) => readonly ParamSpec[];

  /**
   * Builds an `Extractor` from the same options `createParser` takes: a
   * host's own modifier registered under a name this format defines changes
   * what a message naming it says about its value, so an extractor is built
   * the way the parser beside it is. `modifierDefaults` and `onReport` reach
   * nothing — extraction formats nothing and reports nothing.
   */
  export type ExtractorFactory = <Props = {}, Key extends string = Modifier.Key>(options?: Parser.Options<Key, Props>) => Extractor;
}

/**
 * The message as it is written, described rather than resolved. Section 6 is a
 * grammar over the message string and section 7 an escaping rule over the same
 * string; together they say where every character of a message belongs, and
 * that is what this describes — section 8 included, because which characters
 * are padding is a fact about the spelling. Nothing from section 9 on is
 * here: which placeholders bind, what an option selects, what the message
 * resolves to — none of it is a property of the text.
 *
 * It is a *concrete* tree, not an abstract one. A concrete tree describes the
 * source: every character of the message lies in exactly one leaf, the leaves
 * come in the order they are written, and concatenating them spells the
 * message back. There is no abstract tree to offer instead. The tree does have
 * a shape — a placeholder derives inside an option value (section 6, note 10)
 * — but it is the shape the message writes, and an abstract tree would keep
 * what a message means, which is what it resolves to against a payload it has
 * not been given.
 */
export module Cst {
  /** Where a node lies in the message, as `[start, end)` in code units. */
  type Span = { start: number, end: number };

  /** Characters that carry no structural meaning where they stand. */
  export type Text = Span & { type: 'text' };

  /**
   * A backslash and the character it consumes (section 7). `cancels` says
   * which of the two readings the pair takes: a structural meaning cancelled,
   * so that removing the sequence leaves the character alone, or a backslash
   * that denotes itself, so that both characters stand.
   */
  export type Escape = Span & { type: 'escape', cancels: boolean };

  /** Blank padding a name is read without (section 8). */
  export type Space = Span & { type: 'space' };

  /** A delimiter: the pair that opens or closes, or a `:` or `;` that divides. */
  export type Punctuation = Span & { type: 'open' | 'close' | 'separator' };

  /**
   * Something a placeholder writes a name with: the key it selects on, the
   * modifier it names, an option's key. `name` is the span unescaped and
   * `nodes` is how the message spells it.
   *
   * Each answers to that name: a key and a modifier name are matched against
   * something a host wrote, by code-point equality after unescaping (section
   * 6, note 2), and an option key is unescaped the same way though it looks
   * nothing up — the modifier compares it against the value. A name holds no
   * placeholder: one derives in an option value and nowhere else (section 6,
   * note 10).
   *
   * A name may be empty, and an empty one still has a position: a placeholder
   * that names no key carries a `key` node of no width where the key would be.
   */
  export type Name = Span & { type: 'key' | 'modifier' | 'option-key', name: string, nodes: (Text | Escape)[] };

  /**
   * An option's value. It answers to nobody and is not a name: it is message
   * text, so its escape sequences are removed when the message is parsed and a
   * placeholder written in it derives. It carries the subtree rather than a
   * string to compare, and a consumer that wants what the option renders as
   * walks that subtree.
   */
  export type OptionValue = Span & { type: 'option-value', nodes: (Text | Escape | Placeholder)[] };

  /** A construct section 6 derives as a placeholder, and what it is made of. */
  export type Placeholder = Span & { type: 'placeholder', nodes: (Punctuation | Space | Name | OptionValue)[] };

  /**
   * A whole message. Its parts are text and placeholders and nothing else:
   * section 9.1 leaves no third state between them, so a `{{ … }}` construct
   * the grammar does not derive is text here, exactly as it is to resolution.
   * A message that is not text has no characters to describe and carries no
   * parts.
   */
  export type Message = Span & { type: 'message', nodes: (Text | Escape | Placeholder)[] };

  export type Node = Message | Placeholder | Name | OptionValue | Text | Escape | Space | Punctuation;

  /**
   * Describes a message. It reads no options: a name is a name whether or not
   * a modifier answers to it, so nothing a host registers changes the text.
   * This is the build-time and editor-time half of the parser, like
   * `Parser.Extractor` — resolution never calls it.
   */
  export type Parse = (message: Parser.Value) => Message;
}
