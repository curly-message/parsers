// The rows `bench/harness.mjs` measures, each on the build in `bench/dist/`
// that `bench/prepare.sh` writes: `hello` and `hello-intl`, whose stripped
// binaries the size rows weigh; the `.crate` a release ships; and
// `curly-message-bench`, the release build that names the other rows, counts
// the counts and runs the times.
import { execFileSync, spawn } from 'node:child_process';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { gzipSync } from 'node:zlib';

const root = fileURLToPath(new URL('..', import.meta.url));
const dist = (file) => new URL(`dist/${file}`, import.meta.url);
const program = fileURLToPath(dist('curly-message-bench'));

// The harness heads its table with a package's name and version; a crate has
// no package.json to read them from.
export const name = 'curly-message-parser';
export const version = readFileSync(new URL('../Cargo.toml', import.meta.url), 'utf8').match(/^\[package\]\n(?:[^[\n].*\n)*?version = "([^"]+)"/m)[1];

// The crates a consumer's build compiles for the parser, the parser among
// them, as the lockfile resolves them: what a feature adds to every
// consumer's build.
const crates = (...features) => {
  const tree = execFileSync('cargo', ['tree', '--locked', '--offline', '-p', name, '-e', 'normal', '--prefix', 'none', ...features], { cwd: root, encoding: 'utf8' });

  return new Set(tree.split('\n').filter(Boolean).map((line) => line.replace(/ \(\*\)$/, ''))).size;
};

// A time row's program, set up and waiting: each batch is a byte written to
// it, answered by a line once the batch has run. A program that exits fails
// the batch waiting on it and every one after.
const serve = (row) => {
  const child = spawn(program, ['serve', row], { stdio: ['pipe', 'pipe', 'inherit'] });
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
  child.once('exit', (code, signal) => end(new Error(`curly-message-bench exited with ${signal ?? `code ${code}`}`)));
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
  { name: 'curly-message-parser.crate', kind: 'size', run: () => readFileSync(dist('curly-message-parser.crate')).length },
  { name: 'hello: a program resolving one message, stripped', kind: 'size', run: () => readFileSync(dist('hello')).length },
  { name: 'hello-intl: a program resolving through every Intl modifier, stripped', kind: 'size', run: () => readFileSync(dist('hello-intl')).length },
  { name: 'hello-intl, gzipped', kind: 'size', run: () => gzipSync(readFileSync(dist('hello-intl')), { level: 9 }).length },
  { name: 'crates: the parser\'s dependency tree', kind: 'count', run: () => crates() },
  { name: 'crates: the parser\'s dependency tree with intl', kind: 'count', run: () => crates('--features', 'intl') },
  ...listed.map(({ name: row, kind }) => (kind === 'count'
    ? { name: row, kind, run: () => Number(execFileSync(program, ['count', row], { encoding: 'utf8' })) }
    : { name: row, kind, run: async () => {
      served = await serve(row);

      return served.batch;
    }, close: () => served?.close() })),
];
