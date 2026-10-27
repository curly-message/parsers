import { appendFileSync, readFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';

// Rows that fail wherever they share a process with another measurement:
// `node ../harness.mjs --compare .` from here passes only when each side of
// each row, and of each round of a time row, runs in a process of its own,
// when what a row leaves running holds no process open, and when a row's name
// is read as a name even where it reads as an option.
const alone = () => {
  if (globalThis.measured) throw new Error('this process measured something before');

  globalThis.measured = true;
};

// The processes the time row ran in, a line each, in a file named by the
// harness that forked them, so that the row after it can count them: ROUNDS
// pairs, four in the harness. The first row clears what a run cut short left.
const timed = join(tmpdir(), `bench-check-${process.ppid}`);
const PROCESSES = 8;

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

    const processes = new Set(readFileSync(timed, 'utf8').split('\n').filter(Boolean)).size;

    if (processes !== PROCESSES) throw new Error(`the time row ran in ${processes} processes, not ${PROCESSES}`);

    return processes;
  }, close: () => rmSync(timed, { force: true }) },
  { name: '--a size named like an option', kind: 'size', run: () => {
    alone();

    return 1;
  } },
];
