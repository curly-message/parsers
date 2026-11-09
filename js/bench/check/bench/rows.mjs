import { appendFileSync, readFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';

// Rows that fail wherever they share a process with another measurement: the
// harness, run on them by `../check.mjs`, passes only when each side of each
// row, and of each round of a time row, runs in a process of its own, when
// what a row leaves running holds no process open, and when a row's name is
// read as a name even where it reads as an option.
const alone = () => {
  if (globalThis.measured) throw new Error('this process measured something before');

  globalThis.measured = true;
};

// Named here, with no package.json, as a package that npm does not manage is.
export const name = 'bench-check';
export const version = '0.0.0';

// The processes the time row ran in, a line each, in a file named by the
// harness that forked them, so that the row after it can count them: ROUNDS
// per side, four in the harness, each once. The first row clears what a run
// cut short left.
const timed = join(tmpdir(), `bench-check-${process.ppid}`);
const ROUNDS = 4;

export default [
  { name: 'a count', kind: 'count', run: () => {
    alone();
    rmSync(timed, { force: true });

    return 1;
  } },
  { name: 'a size', kind: 'size', run: () => {
    alone();

    return 1;
  } },
  { name: 'a count that leaves a timer running', kind: 'count', run: () => {
    alone();
    setInterval(() => {}, 1000);

    return 1;
  } },
  { name: 'a time', kind: 'time', run: () => {
    alone();
    appendFileSync(timed, `${process.pid}\n`);

    return () => {};
  }, close: () => {} },
  { name: 'the processes the time row ran in', kind: 'count', run: () => {
    alone();

    const lines = readFileSync(timed, 'utf8').split('\n').filter(Boolean);
    const processes = new Set(lines).size;

    if (processes !== lines.length) throw new Error(`the time row ran ${lines.length} times in ${processes} processes`);
    if (!processes || processes % ROUNDS) throw new Error(`the time row ran in ${processes} processes, not a multiple of ${ROUNDS}`);

    return processes;
  }, close: () => rmSync(timed, { force: true }) },
  { name: '--a size named like an option', kind: 'size', run: () => {
    alone();

    return 1;
  } },
];
