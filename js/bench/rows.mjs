// The rows `bench/harness.mjs` measures, each on the build in `dist/`.
import { readFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { gzipSync } from 'node:zlib';
import { cst, createExtractor, createParser } from '../dist/index.js';

// The checker of the package the command runs in, so both sides of a
// comparison count with one TypeScript.
const ts = createRequire(join(process.cwd(), 'package.json'))('typescript');

// Inside the package, so the probe's import reaches the declarations the
// build ships through the package's own `exports`. TypeScript names files
// with forward slashes on every platform.
const probe = fileURLToPath(new URL('../probe.ts', import.meta.url)).replaceAll('\\', '/');
const bundle = () => readFileSync(new URL('../dist/index.js', import.meta.url));

const { resolve } = createParser();
const extract = createExtractor();

// Messages as a catalogue holds them: interpolation, a selection, a plural,
// a placeholder an option value writes, and a formatted number and date.
const MESSAGES = [
  'Hello, {{name}}!',
  '{{name}} has {{count:number}} new {{count:plural; one:message; other:messages}}.',
  '{{gender; male:He; female:She; default:They}} replied to {{thread}}.',
  '{{count:plural; one:{{count}} file; few:{{count}} files; other:{{count}} files}} in {{folder; :the root; default:{{folder}}}}.',
  'Total: {{amount:number}}, due {{due:date}}.',
];

const PAYLOAD = { name: 'Ada', count: 3, gender: 'female', thread: 'Plans', folder: 'docs', amount: 1234.5, due: new Date(Date.UTC(2026, 0, 31)) };

const nested = (depth) => `${'{{v; a:'.repeat(depth)}x${'}}'.repeat(depth)}`;
const flat = (count) => Array.from({ length: count }, (_, index) => `{{p${index}}}`).join(' ');

// What resolving the message set reads of its payload: every trap a read can
// go through, counted.
const reads = () => {
  let count = 0;
  const handler = Object.fromEntries(['get', 'has', 'getOwnPropertyDescriptor', 'ownKeys', 'getPrototypeOf'].map((trap) => [trap, (...args) => {
    count += 1;

    return Reflect[trap](...args);
  }]));
  const payload = new Proxy(PAYLOAD, handler);

  for (const message of MESSAGES) resolve(message, { payload, locale: 'en' });

  return count;
};

// What checking resolve calls against a payload type of 20 keys costs a
// consumer's checker, against the declarations the build ships. The checker
// carries what it works out for one call over to the next, so the count
// follows the payload type rather than the number of calls.
const instantiations = () => {
  const names = Array.from({ length: 20 }, (_, index) => `key${index}`);
  const payload = `{ ${names.map((name) => `${name}: 'value'`).join(', ')} }`;
  const text = [
    "import { createParser } from '@curly-message/parser';",
    `type Payload = { ${names.map((name) => `${name}: string;`).join(' ')} };`,
    'const { resolve } = createParser<Payload>();',
    ...names.map((name) => `resolve('{{${name}}}', { payload: ${payload} });`),
  ].join('\n');
  const options = {
    module: ts.ModuleKind.ESNext,
    moduleResolution: ts.ModuleResolutionKind.Bundler,
    noEmit: true,
    skipLibCheck: true,
    strict: true,
    target: ts.ScriptTarget.ES2022,
    types: [],
  };
  const host = ts.createCompilerHost(options);
  const fileExists = host.fileExists.bind(host);
  const readFile = host.readFile.bind(host);
  const getSourceFile = host.getSourceFile.bind(host);

  host.fileExists = (name) => name === probe || fileExists(name);
  host.readFile = (name) => (name === probe ? text : readFile(name));
  host.getSourceFile = (name, version, ...rest) => (name === probe ? ts.createSourceFile(name, text, version) : getSourceFile(name, version, ...rest));

  const program = ts.createProgram([probe], options, host);
  const diagnostics = ts.getPreEmitDiagnostics(program);

  if (diagnostics.length) throw new Error(ts.flattenDiagnosticMessageText(diagnostics[0].messageText, '\n'));

  return program.getInstantiationCount();
};

export default [
  { name: 'dist/index.js', kind: 'size', run: () => bundle().length },
  { name: 'dist/index.js, gzipped', kind: 'size', run: () => gzipSync(bundle(), { level: 9 }).length },
  { name: 'payload reads: resolving five catalogue messages', kind: 'count', run: reads },
  { name: 'checker: instantiations of resolve calls against a payload type of 20 keys', kind: 'count', run: instantiations },
  { name: 'resolve: five catalogue messages', kind: 'time', run: () => () => {
    for (const message of MESSAGES) resolve(message, { payload: PAYLOAD, locale: 'en' });
  } },
  { name: 'resolve: 5 000 placeholders', kind: 'time', run: () => {
    const message = flat(5000);

    return () => resolve(message, { payload: {}, locale: 'en' });
  } },
  { name: 'resolve: nested 10 000 levels', kind: 'time', run: () => {
    const message = nested(10000);

    return () => resolve(message, { payload: { v: 'a' }, locale: 'en' });
  } },
  { name: 'cst: five catalogue messages', kind: 'time', run: () => () => {
    for (const message of MESSAGES) cst(message);
  } },
  { name: 'cst: 5 000 placeholders', kind: 'time', run: () => {
    const message = flat(5000);

    return () => cst(message);
  } },
  { name: 'cst: nested 250 levels', kind: 'time', run: () => {
    const message = nested(250);

    return () => cst(message);
  } },
  { name: 'createExtractor: five catalogue messages', kind: 'time', run: () => () => {
    for (const message of MESSAGES) extract(message);
  } },
  { name: 'createExtractor: nested 250 levels', kind: 'time', run: () => {
    const message = nested(250);

    return () => extract(message);
  } },
];
