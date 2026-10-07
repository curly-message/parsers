import type * as Shipped from '@curly-message/parser';
import type * as Source from '../../src/index';

// The declarations the build ships are the ones the source declares: a type
// the bundler widened, dropped to `any` or rewrote fails to compile here. A
// generic is compared instantiated, because TypeScript never counts two
// declarations of a deferred conditional type as identical. A new export
// joins the list; one left out of it goes unchecked.
type Equal<A, B> = (<T>() => T extends A ? 1 : 2) extends (<T>() => T extends B ? 1 : 2) ? true : false;
type Assert<T extends true> = T;

type Payload = { name: string; count: number };
type Props = { tone: 'formal' };

export type Surface = [
  Assert<Equal<typeof Shipped.createParser<Payload>, typeof Source.createParser<Payload>>>,
  Assert<Equal<typeof Shipped.createParser<Payload, Props, 'shout'>, typeof Source.createParser<Payload, Props, 'shout'>>>,
  Assert<Equal<typeof Shipped.createExtractor<Props>, typeof Source.createExtractor<Props>>>,
  Assert<Equal<typeof Shipped.createExtractor<Props, 'shout'>, typeof Source.createExtractor<Props, 'shout'>>>,
  Assert<Equal<typeof Shipped.cst, typeof Source.cst>>,
  Assert<Equal<Shipped.Report, Source.Report>>,
  Assert<Equal<Shipped.Locale, Source.Locale>>,
];
