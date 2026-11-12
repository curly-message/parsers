// The rows `bench/harness.mjs` measures, each on the wheel in `dist/` as
// `.bench-venv` installs it (`uv run --no-project tasks.py bench-venv`).
//
// The rows that run inside the interpreter are `bench/serve.py`'s, which
// lists them; each is measured in a Python process of its own, started once
// per measuring process and stopped by the row's `close`. A row of a process's
// whole life (an import, a first Intl call) starts a fresh interpreter per run.
import { execFileSync, spawn } from 'node:child_process';
import { existsSync, readdirSync, statSync } from 'node:fs';
import { createInterface } from 'node:readline';
import { fileURLToPath } from 'node:url';

const here = (path) => fileURLToPath(new URL(path, import.meta.url));
const python = here(process.platform === 'win32' ? '../.bench-venv/Scripts/python.exe' : '../.bench-venv/bin/python');
const serve = here('./serve.py');
const dist = here('../dist');

if (!existsSync(python)) throw new Error('.bench-venv holds no interpreter: run `uv run --no-project tasks.py bench-venv` first');

// `-I` keeps the source tree, the user's site directory and PYTHON* off the
// import, so what is imported is the wheel the venv installed.
const listed = JSON.parse(execFileSync(python, ['-I', serve, '--list'], { encoding: 'utf8' }));

const wheel = () => {
  const wheels = readdirSync(dist).filter((name) => name.endsWith('.whl'));

  if (wheels.length !== 1) throw new Error(`dist/ holds ${wheels.length} wheels, not one`);

  return statSync(`${dist}/${wheels[0]}`).size;
};

// One interpreter answering one line per request, in order.
const server = () => {
  const child = spawn(python, ['-I', serve], { stdio: ['pipe', 'pipe', 'inherit'] });
  const waiting = [];
  let gone;

  createInterface({ input: child.stdout }).on('line', (line) => waiting.shift()?.settle(JSON.parse(line)));
  child.once('exit', (code, signal) => {
    gone = new Error(`serve.py exited with ${signal ?? `code ${code}`}`);
    for (const { fail } of waiting.splice(0)) fail(gone);
  });
  child.once('error', (error) => {
    gone = error;
    for (const { fail } of waiting.splice(0)) fail(error);
  });

  return {
    ask: (request) => new Promise((settle, fail) => {
      if (gone) return fail(gone);

      waiting.push({ settle: (reply) => (reply.error ? fail(new Error(reply.error)) : settle(reply)), fail });
      child.stdin.write(`${JSON.stringify(request)}\n`);
    }),
    stop: () => new Promise((settle) => {
      if (gone) return settle();

      child.once('exit', () => settle());
      child.stdin.end();
    }),
  };
};

// What a fresh interpreter takes to run `code` and exit.
const lifetime = (code) => () => () => new Promise((settle, fail) => {
  const child = spawn(python, ['-I', '-c', code], { stdio: 'inherit' });

  child.once('error', fail);
  child.once('exit', (status, signal) => (status === 0 ? settle() : fail(new Error(`python exited with ${signal ?? `code ${status}`}`))));
});

const servers = new Map();

const served = ({ name, kind }) => (kind === 'time'
  ? {
    name,
    kind,
    run: async () => {
      const side = server();

      servers.set(name, side);
      await side.ask({ row: name });

      return () => side.ask({ run: true });
    },
    close: () => servers.get(name)?.stop(),
  }
  : {
    name,
    kind,
    run: async () => {
      const side = server();

      try {
        return (await side.ask({ row: name })).value;
      } finally {
        await side.stop();
      }
    },
  });

export default [
  { name: 'dist/*.whl', kind: 'size', run: wheel },
  ...listed.map(served),
  { name: 'import curly_message, in a fresh interpreter', kind: 'time', run: lifetime('import curly_message') },
  {
    name: 'first Intl call, in a fresh interpreter',
    kind: 'time',
    run: lifetime("import curly_message; assert curly_message.create_parser().resolve('{{n:number}}', {'n': 1234.5}, locale='en') == '1,234.5'"),
  },
];
