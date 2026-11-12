// The harness beside this directory, run on the rows here as CI runs it before
// a benchmark: compared with this package, run alone, and given no base, a
// base that does not exist, one without rows, one whose rows fail to load, and
// one whose package.json names no version. It prints each case and the
// harness's output where one fails.
//
//   node check.mjs
import { spawnSync } from 'node:child_process';
import { copyFileSync, mkdirSync, mkdtempSync, realpathSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const here = dirname(fileURLToPath(import.meta.url));
const temp = mkdtempSync(join(tmpdir(), 'bench-base-'));
let failed = 0;

// `--compare .` means this directory only where INIT_CWD, which npm sets, is
// not set.
const harness = (args) => {
  const { status, stdout, stderr, error } = spawnSync(process.execPath, [join(here, '..', 'harness.mjs'), ...args], {
    cwd: here,
    env: { ...process.env, INIT_CWD: undefined },
    encoding: 'utf8',
  });

  return { status, error, output: `${stdout ?? ''}${stderr ?? ''}`, lines: (stdout ?? '').split('\n') };
};

// The cells of the table's line that starts with `name`, if there is one.
const cells = (lines, name) => lines.find((line) => line.startsWith(`| ${name} |`))?.split('|').slice(1, -1).map((text) => text.trim());

const check = (title, args, expected) => {
  const run = harness(args);
  const missed = [...(run.error ? [`it ran: ${run.error.message}`] : []), ...expected(run).filter(([, holds]) => !holds).map(([what]) => what)];

  if (!missed.length) return console.log(`Passed: ${title}.`);

  failed += 1;
  console.log(`Failed: ${title}: expected ${missed.join('; ')}.\n\n${run.output}`);
};

const COUNTED = 'the processes the time row ran in';

try {
  check('compared with this package', ['--compare', '.'], ({ status, lines }) => [
    ['exit 0', status === 0],
    ['the heading to name bench-check', lines.includes('## Benchmark of bench-check')],
    ['the base and the change to read 0.0.0', cells(lines, 'Row')?.slice(2, 4).join() === 'Base 0.0.0,Change 0.0.0'],
    ['the base to be named by its path', lines.includes(`Compared with the base at \`${realpathSync(here)}\`.`)],
    ['8 processes on each side', cells(lines, COUNTED)?.slice(2, 4).join() === '8,8'],
    ['a pass', lines.includes('Passed.')],
  ]);

  check('run alone', [], ({ status, lines }) => [
    ['exit 0', status === 0],
    ['no base', !lines.some((line) => line.startsWith('Compared with'))],
    ['4 processes', cells(lines, COUNTED)?.[2] === '4'],
    ['a pass that compared nothing', lines.includes('Passed; nothing was compared.')],
  ]);

  const missing = join(temp, 'missing');
  const rowless = join(temp, 'rowless');
  const broken = join(temp, 'broken');
  const unversioned = join(temp, 'unversioned');

  mkdirSync(rowless);
  writeFileSync(join(rowless, 'package.json'), JSON.stringify({ name: 'bench-check', version: '0.0.0', type: 'module' }));
  mkdirSync(join(broken, 'bench'), { recursive: true });
  writeFileSync(join(broken, 'bench', 'rows.mjs'), 'throw new Error(\'a base that fails to load\');\n');
  mkdirSync(join(unversioned, 'bench'), { recursive: true });
  copyFileSync(join(here, 'bench', 'rows.mjs'), join(unversioned, 'bench', 'rows.mjs'));
  writeFileSync(join(unversioned, 'package.json'), JSON.stringify({ name: 'bench-check', type: 'module' }));

  for (const accepted of [[], ['--accepted']]) {
    check(`given no base${accepted.length ? ', accepted' : ''}`, ['--compare', '', ...accepted], ({ status, lines }) => [
      ['exit 1', status === 1],
      ['no base to be named', lines.includes('Failed: `--compare` names no base.')],
      ['no row to run', !cells(lines, COUNTED)],
    ]);

    check(`given a base that does not exist${accepted.length ? ', accepted' : ''}`, ['--compare', missing, ...accepted], ({ status, lines }) => [
      ['exit 1', status === 1],
      ['the base to be missing', lines.includes(`Failed: the base at \`${missing}\` does not exist.`)],
      ['no row to run', !cells(lines, COUNTED)],
    ]);

    check(`given a base without rows${accepted.length ? ', accepted' : ''}`, ['--compare', rowless, ...accepted], ({ status, lines }) => [
      ['exit 1', status === 1],
      ['the base to have no rows', lines.includes(`Failed: the base at \`${rowless}\` has no bench/rows.mjs.`)],
      ['no row to run', !cells(lines, COUNTED)],
    ]);
  }

  check('given a base whose rows fail to load', ['--compare', broken], ({ status, lines }) => [
    ['exit 1', status === 1],
    ['the base to be named by its path', lines.includes(`The base at \`${broken}\` failed to load its rows: a base that fails to load`)],
    ['a failure the label accepts', lines.some((line) => line.startsWith('Failed: 1 row grew,'))],
  ]);

  check('given a base whose rows fail to load, accepted', ['--compare', broken, '--accepted'], ({ status, lines }) => [
    ['exit 0', status === 0],
    ['an accepted pass', lines.includes('Passed: 1 row grew, went missing, changed kind or failed on the base, accepted.')],
  ]);

  check('given a base whose package.json names no version', ['--compare', unversioned], ({ status, lines }) => [
    ['exit 0', status === 0],
    ['the base to read 0.0.0', cells(lines, 'Row')?.[2] === 'Base 0.0.0'],
    ['a pass', lines.includes('Passed.')],
  ]);
} finally {
  rmSync(temp, { recursive: true, force: true });
}

process.exitCode = failed ? 1 : 0;
