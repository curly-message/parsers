// The benchmark: what a build costs, row by row. A row is a count, a size or a
// time, and `bench/rows.mjs` of the package the command runs in lists them.
//
//   node bench/harness.mjs [--compare <dir>] [--accepted]
//
// With `--compare`, the rows of the package checked out and built at `<dir>`
// measure that build, and each is set beside the row of the same name here.
// Each side's rows read its own build, so a row survives a change to the API
// it calls. A count is deterministic and gates: one that grew fails the run,
// as do a row gone missing, a row whose kind changed, a base whose rows fail
// to load and a row of the base that fails, unless `--accepted` says the
// change means them. This package fails the run regardless when its
// `bench/rows.mjs` is missing or fails to load, or when a row of it fails. A
// size that grew and a time beyond its spread are flagged for review.
//
// A time is per run of what the row measures, sampled base and change by turn
// in this one process, so both sides see the same machine. The spread leaves
// out the lowest and the highest quarter of the samples, rounded down, so a
// busy neighbour neither hides nor flags a change.
//
// This file is the same in every repository of the family; change every copy
// at once.
import { existsSync, readFileSync } from 'node:fs';
import { arch, platform } from 'node:os';
import { resolve } from 'node:path';
import { pathToFileURL } from 'node:url';
import { parseArgs } from 'node:util';

const KINDS = ['count', 'size', 'time'];
const SAMPLES = 20;
const SAMPLE_MS = 10;
const WARMUP_MS = 50;

const { values: args } = parseArgs({
  options: {
    compare: { type: 'string' },
    accepted: { type: 'boolean', default: false },
  },
});

const manifest = (dir) => JSON.parse(readFileSync(resolve(dir, 'package.json'), 'utf8'));

const failure = (error) => ({ error: error instanceof Error ? error.message : String(error) });

// A side's rows by name, or why there are none to read. A base checked out
// before the package had a benchmark has none, and is no failure; a directory
// that holds no package is.
const load = async (dir) => {
  if (!existsSync(resolve(dir, 'package.json'))) return { error: 'no package.json is there' };

  const file = resolve(dir, 'bench/rows.mjs');

  if (!existsSync(file)) return { absent: true };

  try {
    const { default: rows } = await import(pathToFileURL(file).href);
    const named = new Map();

    for (const row of rows) {
      if (typeof row?.name !== 'string' || !row.name) throw new Error('a row has no name');
      if (!KINDS.includes(row.kind)) throw new Error(`the row \`${row.name}\` is of kind ${String(row.kind)}, not one of ${KINDS.join(', ')}`);
      if (typeof row.run !== 'function') throw new Error(`the row \`${row.name}\` has no run`);
      if (named.has(row.name)) throw new Error(`two rows are named \`${row.name}\``);

      named.set(row.name, row);
    }

    return { rows: named };
  } catch (error) {
    return failure(error);
  }
};

const runs = (op, count) => {
  const start = performance.now();

  for (let index = 0; index < count; index += 1) op();

  return performance.now() - start;
};

// How many runs one sample takes to last at least SAMPLE_MS, after the
// engine has had WARMUP_MS to optimize what it runs.
const calibrate = (op) => {
  const start = performance.now();

  do {
    op();
  } while (performance.now() - start < WARMUP_MS);

  let count = 1;

  while (runs(op, count) < SAMPLE_MS) count *= 2;

  return count;
};

const summary = (samples) => {
  const sorted = [...samples].sort((a, b) => a - b);
  const cut = Math.floor(sorted.length / 4);
  const middle = sorted.length / 2;
  const median = sorted.length % 2 ? sorted[Math.floor(middle)] : (sorted[middle - 1] + sorted[middle]) / 2;

  return { value: median, low: sorted[cut], high: sorted[sorted.length - 1 - cut] };
};

// Each side's time per run. The sides take turns sample by sample, the side
// that goes first alternating, so a drift in the machine reaches both alike.
const time = (ops) => {
  const results = ops.map(() => undefined);
  const counts = ops.map((op, side) => {
    try {
      return calibrate(op);
    } catch (error) {
      results[side] = failure(error);

      return 0;
    }
  });
  const samples = ops.map(() => []);

  for (let index = 0; index < SAMPLES; index += 1) {
    const order = index % 2 ? [...ops.keys()].reverse() : [...ops.keys()];

    for (const side of order) {
      if (results[side]) continue;

      try {
        samples[side].push(runs(ops[side], counts[side]) / counts[side]);
      } catch (error) {
        results[side] = failure(error);
      }
    }
  }

  return results.map((result, side) => result ?? summary(samples[side]));
};

const measure = async (row) => {
  try {
    const value = await row.run();

    if (row.kind === 'time') return typeof value === 'function' ? value : failure(new Error('a time row returns the function it times'));

    return Number.isFinite(value) ? { value } : failure(new Error(`a ${row.kind} row returns a number, not ${String(value)}`));
  } catch (error) {
    return failure(error);
  }
};

// Every row of either side: this package's in its order, then the ones only
// the base has. A row whose kind differs between the sides is measured on this
// package's side alone, since a count and a time do not compare.
const measureAll = async (change, base) => {
  const names = [...change.keys(), ...[...(base?.keys() ?? [])].filter((name) => !change.has(name))];
  const results = [];

  for (const name of names) {
    const sides = [base?.get(name), change.get(name)];
    const kind = (sides[1] ?? sides[0]).kind;
    const was = sides[0] && sides[1] && sides[0].kind !== kind ? sides[0].kind : undefined;

    if (was) sides[0] = undefined;

    const measured = await Promise.all(sides.map((row) => (row ? measure(row) : undefined)));
    const ops = measured.map((result) => (typeof result === 'function' ? result : undefined));
    const timed = time(ops.filter(Boolean));

    for (const side of [0, 1]) {
      if (ops[side]) measured[side] = timed.shift();
    }

    results.push({ name, kind, was, base: measured[0], change: measured[1] });
  }

  return results;
};

const UNITS = [['s', 1e3], ['ms', 1], ['\u00b5s', 1e-3], ['ns', 1e-6]];

const duration = (ms) => {
  const [unit, size] = UNITS.find(([, size]) => ms >= size) ?? UNITS[UNITS.length - 1];

  return `${Number((ms / size).toPrecision(3))} ${unit}`;
};

const formatted = (kind, result) => {
  if (!result) return '\u2014';
  if (result.error) return 'failed';
  if (kind === 'time') return `${duration(result.value)} (${duration(result.low)}\u2013${duration(result.high)})`;

  return `${result.value.toLocaleString('en-US')}${kind === 'size' ? ' B' : ''}`;
};

// What a row's comparison says, and whether it fails the run.
const verdict = ({ kind, was, base, change }, compared) => {
  if (change?.error) return { note: `failed: ${change.error}`, fatal: true };
  if (!compared) return { note: '' };
  if (!change) return { note: 'missing', gated: true };
  if (was) return { note: `was a ${was}`, gated: true };
  if (base?.error) return { note: `base failed: ${base.error}`, gated: true };
  if (!base) return { note: 'new' };

  const delta = base.value === 0 ? 'from 0' : `${change.value >= base.value ? '+' : ''}${(((change.value - base.value) / base.value) * 100).toFixed(1)}%`;

  if (kind === 'time') {
    if (change.low > base.high) return { note: `slower, ${delta}` };
    if (change.high < base.low) return { note: `faster, ${delta}` };

    return { note: '' };
  }

  if (change.value > base.value) return { note: `grew, ${delta}`, gated: kind === 'count' };
  if (change.value < base.value) return { note: `fell, ${delta}` };

  return { note: '' };
};

const cell = (text) => text.replaceAll('|', '\\|').replaceAll('\n', ' ');

const here = manifest('.');
const change = await load('.');
const lines = [`## Benchmark of ${here.name}`, ''];

// Written where the table would be, so a PR's comment says why there is none.
if (!change.rows) {
  console.log([...lines, `Failed: ${change.error ? `this package's rows failed to load: ${change.error}` : 'this package has no bench/rows.mjs'}.`].join('\n'));
  process.exit(1);
}

// A path is meant from where the command was given: npm runs a workspace's
// script in the workspace and names that place in INIT_CWD.
const against = args.compare === undefined ? undefined : resolve(process.env.INIT_CWD ?? '.', args.compare);
const base = against === undefined ? undefined : await load(against);
const compared = Boolean(base?.rows);

if (base?.error) {
  lines.push(`The base at \`${args.compare}\` failed to load its rows: ${base.error}`, '');
} else if (base?.absent) {
  lines.push(`The base at \`${args.compare}\` has no benchmark, so nothing is compared.`, '');
}

const results = await measureAll(change.rows, base?.rows);
const verdicts = results.map((result) => verdict(result, compared));

lines.push(
  `Node ${process.version}, ${platform()} ${arch()}. A time is per run: the median of ${SAMPLES} samples, and the range of the samples without their fastest and slowest quarter.`,
  '',
);

if (compared) {
  lines.push(`| Row | Kind | Base ${manifest(against).version} | Change ${here.version} | |`, '|---|---|--:|--:|---|');
  results.forEach((result, index) => lines.push(`| ${cell(result.name)} | ${result.kind} | ${formatted(result.kind, result.base)} | ${formatted(result.kind, result.change)} | ${cell(verdicts[index].note)} |`));
} else {
  lines.push(`| Row | Kind | ${here.version} | |`, '|---|---|--:|---|');
  results.forEach((result, index) => lines.push(`| ${cell(result.name)} | ${result.kind} | ${formatted(result.kind, result.change)} | ${cell(verdicts[index].note)} |`));
}

const fatal = verdicts.filter(({ fatal }) => fatal).length;
const gated = verdicts.filter(({ gated }) => gated).length + (base?.error ? 1 : 0);

lines.push('');

if (fatal) lines.push(`Failed: ${fatal} row${fatal > 1 ? 's' : ''} of this package failed.`);
else if (gated && !args.accepted) lines.push(`Failed: ${gated} row${gated > 1 ? 's' : ''} grew, went missing, changed kind or failed on the base. A change that means it is accepted with \`--accepted\` (the \`bench-accepted\` label on a PR).`);
else if (gated) lines.push(`Passed: ${gated} row${gated > 1 ? 's' : ''} grew, went missing, changed kind or failed on the base, accepted.`);
else lines.push('Passed.');

console.log(lines.join('\n'));
process.exitCode = fatal || (gated && !args.accepted) ? 1 : 0;
