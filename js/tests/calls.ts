/**
 * How many times `run` calls the given methods. A read of a message goes
 * through them, so the count says what the read costs, the same on every
 * machine and runtime where a time would not. Each method is replaced only
 * while `run` runs, by a counter that keeps none of the calls' arguments.
 */
export const calls = (methods: readonly (readonly [object, PropertyKey])[], run: () => unknown) => {
  let count = 0;
  const replaced = methods.map(([target, key]) => [target, key, Object.getOwnPropertyDescriptor(target, key)!] as const);

  for (const [target, key, descriptor] of replaced) {
    const original = descriptor.value as (...args: unknown[]) => unknown;

    Object.defineProperty(target, key, {
      ...descriptor,
      value(this: unknown, ...args: unknown[]) {
        count += 1;

        return original.apply(this, args);
      },
    });
  }

  try {
    run();
  } finally {
    for (const [target, key, descriptor] of replaced) Object.defineProperty(target, key, descriptor);
  }

  return count;
};

/**
 * The methods a message is read through one code unit at a time: the units
 * themselves, and the test of each against the line terminators. A count of
 * them pins how the package reads, so a correct change that reads a unit some
 * other way can lower it; one that reads more grows it.
 */
export const READS = [
  [String.prototype, 'charAt'],
  [String.prototype, 'charCodeAt'],
  [String.prototype, 'codePointAt'],
  [RegExp.prototype, 'test'],
] as const;
