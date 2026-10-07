// What the type tests compile against in place of vitest, whose own
// declarations do not compile at `skipLibCheck: false`: the subject here is
// this package's declarations, and the suite's calls into vitest are typed
// only as far as compiling them needs.
type Assertion = { not: Assertion } & Record<string, (...expected: unknown[]) => void>;

export declare function describe(name: string, body: () => void): void;
export declare function it(name: string, body: () => void | Promise<void>): void;
export declare function expect(actual: unknown): Assertion;
