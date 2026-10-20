// The rows `bench/harness.mjs` measures, each on the build in `dist/` that
// `bench/prepare.sh` writes: `dist/size`, a program resolving one message,
// whose stripped binary the size rows weigh, and `dist/bench`, the Go
// program that names the other rows, counts the counts and runs the times.
import { execFileSync, spawn } from 'node:child_process';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { gzipSync } from 'node:zlib';

const program = fileURLToPath(new URL('../dist/bench', import.meta.url));
const binary = () => readFileSync(new URL('../dist/size', import.meta.url));

// A time row's program, set up and waiting: each batch is a byte written to
// it, answered by a byte once the batch has run. A program that exits fails
// the batch waiting on it and every one after.
const serve = (name) => {
  const child = spawn(program, ['serve', name], { stdio: ['pipe', 'pipe', 'inherit'] });
  const waiting = [];
  let gone;
  const exited = new Promise((settle) => child.once('close', settle));
  const end = (error) => {
    gone ??= error;
    for (const { reject } of waiting.splice(0)) reject(gone);
  };
  const reply = () => (gone ? Promise.reject(gone) : new Promise((resolve, reject) => waiting.push({ resolve, reject })));

  child.stdout.on('data', (chunk) => {
    for (let index = 0; index < chunk.length; index += 1) waiting.shift()?.resolve();
  });
  child.once('error', end);
  child.once('exit', (code, signal) => end(new Error(`dist/bench exited with ${signal ?? `code ${code}`}`)));
  // A write to a program that has exited fails here as well; the exit says why.
  child.stdin.on('error', () => {});

  return reply().then(() => ({
    batch: () => {
      const answer = reply();

      child.stdin.write('\n');

      return answer;
    },
    close: () => {
      child.stdin.end();

      return exited;
    },
  }));
};

const listed = execFileSync(program, ['list'], { encoding: 'utf8' }).split('\n').filter(Boolean).map((line) => JSON.parse(line));

// The row this process serves: the harness calls a row's `run` once in each
// process that measures it.
let served;

export default [
  { name: 'dist/size: a program resolving one message, stripped', kind: 'size', run: () => binary().length },
  { name: 'dist/size, gzipped', kind: 'size', run: () => gzipSync(binary(), { level: 9 }).length },
  ...listed.map(({ name, kind }) => (kind === 'count'
    ? { name, kind, run: () => Number(execFileSync(program, ['count', name], { encoding: 'utf8' })) }
    : { name, kind, run: async () => {
      served = await serve(name);

      return served.batch;
    }, close: () => served?.close() })),
];
