// The rows `bench/harness.mjs` measures, each on what the package ships: the
// source in `src/`, run by PHP through the package's own autoloader.
//
// A time row runs `bench/worker.php` as one long-lived process with opcache
// on and JIT off, which is how PHP serves requests under FPM by default. The
// worker builds the row's input once and, per sample the harness asks for,
// runs the row's operation the number of times the row's name gives, so the
// round trip to it is a small part of what is timed, and the same on both
// sides of a comparison. Each name says what PHP measured it, since that is
// the PHP on the PATH rather than anything the package holds.
import { execFileSync, spawn } from 'node:child_process';
import { readdirSync, readFileSync, statSync } from 'node:fs';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { gzipSync } from 'node:zlib';

const PHP = 'php';
const OPCACHE = ['-d', 'opcache.enable_cli=1', '-d', 'opcache.jit=disable'];
const worker = fileURLToPath(new URL('worker.php', import.meta.url));
const src = fileURLToPath(new URL('../src/', import.meta.url));

const configuration = JSON.parse(execFileSync(PHP, [...OPCACHE, worker, '--configuration'], { encoding: 'utf8' }));
const runtime = `PHP ${configuration.php}, opcache ${configuration.opcache ? 'on' : 'off'}, JIT ${configuration.jit ? 'on' : 'off'}`;
const intl = `${runtime}, ${configuration.icu === null ? 'no ICU' : `ICU ${configuration.icu}`}`;

// The source as the archive a release ships holds it, file by file in the
// order of their paths, so the size reads the same on every platform.
const source = () => Buffer.concat(readdirSync(src, { recursive: true })
  .map((path) => String(path).replaceAll('\\', '/'))
  .filter((path) => statSync(join(src, path)).isFile())
  .sort()
  .map((path) => readFileSync(join(src, path))));

const count = (row) => Number(execFileSync(PHP, [worker, '--count', row], { encoding: 'utf8' }));

// A worker for one row, and the function that times a batch on it.
const timed = (row, times) => {
  let child;
  let answers;

  return {
    run: async () => {
      child = spawn(PHP, [...OPCACHE, worker, row, String(times)], { stdio: ['pipe', 'pipe', 'inherit'] });
      child.stdout.setEncoding('utf8');

      const waiting = [];
      let buffered = '';
      const exited = new Promise((settle) => child.once('exit', (code, signal) => settle(new Error(`the worker exited with ${signal ?? `code ${code}`}`))));

      exited.then((error) => waiting.splice(0).forEach(({ reject }) => reject(error)));
      child.stdout.on('data', (chunk) => {
        buffered += chunk;

        for (let end = buffered.indexOf('\n'); end !== -1; end = buffered.indexOf('\n')) {
          buffered = buffered.slice(end + 1);
          waiting.shift()?.resolve();
        }
      });
      answers = () => new Promise((resolve, reject) => {
        if (child.exitCode !== null || child.signalCode !== null) reject(new Error('the worker has exited'));
        else waiting.push({ resolve, reject });
      });

      // The worker answers once it has built its input and run it once.
      await answers();

      return () => {
        const answered = answers();

        child.stdin.write('\n');

        return answered;
      };
    },
    close: () => new Promise((settle) => {
      if (!child || child.exitCode !== null || child.signalCode !== null) return settle();

      child.once('exit', () => settle());
      child.stdin.end();
    }),
  };
};

const time = (name, row, times, label = runtime) => ({ name: `${name}, x ${times} (${label})`, kind: 'time', ...timed(row, times) });

export default [
  { name: 'src/', kind: 'size', run: () => source().length },
  { name: 'src/, gzipped', kind: 'size', run: () => gzipSync(source(), { level: 9 }).length },
  { name: 'host calls: resolving five catalogue messages over objects', kind: 'count', run: () => count('host-calls') },
  { name: 'modifier calls: resolving the catalogue and three messages a host modifier is on', kind: 'count', run: () => count('modifier-calls') },
  time('resolve: five catalogue messages', 'catalogue', 10, intl),
  time('resolve: 5 000 placeholders', 'placeholders', 1),
  time('resolve: nested 10 000 levels', 'nested', 1),
  time('resolve: a fallback chain nested 10 000 levels', 'chain', 1),
  time('resolve: a placeholder of 10 000 blanks', 'blanks', 1),
  time('resolve: a key with 10 000 blanks inside', 'key-blanks', 10),
  time('resolve: 1 000 placeholders that never close', 'unclosed', 1),
  time('resolve: 5 000 options', 'options', 1),
  time('resolve: 5 000 placeholders in a fiber, beside 250 suspended holding options', 'parked', 1),
  time('resolve: an option key of 50 000 bytes', 'option-key', 1),
  time('resolve: wrapper props of 5 000 entries', 'wrapper', 10),
  time('resolve: 5 000 keys beginning with NUL, over an object of 5 000 entries', 'nul-keys', 1),
  time('resolve: 1 000 comparisons of text outside ASCII', 'comparisons', 1),
  time('resolve: a list of 1 000 records, serialized', 'serialized', 10),
  time('resolve: a list no conversion describes, read at 1 000 placeholders', 'reread', 1),
  time('resolve: a list with a hole, past what a conversion visits, read at 1 000 keys', 'holed', 1),
  time('resolve: an object past what a conversion visits, in a list read at 100 keys', 'wide', 1),
  time('resolve: 5 000 placeholders whose host modifier reads an option and the chain, in a fiber', 'hosted-reads', 1),
  time('Cst::parse: five catalogue messages', 'cst-catalogue', 10),
  time('Cst::parse: 5 000 placeholders', 'cst-placeholders', 1),
  time('Cst::parse: nested 250 levels', 'cst-nested', 1),
  time('resolve: five catalogue messages at cs, formatters kept', 'intl-warm', 10, intl),
  time('resolve: five catalogue messages at cs, formatters built afresh', 'intl-cold', 10, intl),
  time('resolve: a number at cs, formatters built afresh', 'number-cold', 10, intl),
  time('resolve: a date at cs, formatters built afresh', 'date-cold', 10, intl),
  time('resolve: a relative time at cs, formatters built afresh', 'ago-cold', 10, intl),
];
