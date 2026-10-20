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

const { resolve, compile } = createParser();
const { resolve: resolveWrapped } = createParser({ customModifiers: { test: ({ value }) => value } });
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

// Messages a catalogue holds as plain text, with no placeholder to resolve.
const PLAIN = [
  'Save changes',
  'Your changes have been saved.',
  'Delete this folder? Everything in it is deleted too, and this cannot be undone.',
  'Signed in',
  'No results match your search. Try fewer or different words.',
];

const PAYLOAD = { name: 'Ada', count: 3, gender: 'female', thread: 'Plans', folder: 'docs', amount: 1234.5, due: new Date(Date.UTC(2026, 0, 31)) };

// A catalogue of 2 000 distinct messages, more than a parser keeps.
const CATALOGUE = Array.from({ length: 2000 }, (_, index) => `${MESSAGES[index % MESSAGES.length]} (${index})`);

const nested = (depth) => `${'{{v; a:'.repeat(depth)}x${'}}'.repeat(depth)}`;
const flat = (count) => Array.from({ length: count }, (_, index) => `{{p${index}}}`).join(' ');

// A first resolution, through a parser that has seen no message yet, so a
// row of an input's shape times the reading of it and not what a parser may
// keep of it from the call before.
const first = (message, payload) => () => createParser().resolve(message, { payload, locale: 'en' });

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
  { name: 'resolve: five catalogue messages of plain text', kind: 'time', run: () => () => {
    for (const message of PLAIN) resolve(message, { payload: PAYLOAD, locale: 'en' });
  } },
  { name: 'resolve: five catalogue messages, compiled', kind: 'time', run: () => {
    const compiled = MESSAGES.map((message) => compile(message));

    return () => {
      for (const message of compiled) message.resolve({ payload: PAYLOAD, locale: 'en' });
    };
  } },
  { name: 'resolve: 2 000 catalogue messages, more than a parser keeps', kind: 'time', run: () => {
    const parser = createParser();

    return () => {
      for (const message of CATALOGUE) parser.resolve(message, { payload: PAYLOAD, locale: 'en' });
    };
  } },
  { name: 'resolve: 2 000 catalogue messages, compiled', kind: 'time', run: () => {
    const parser = createParser();
    const compiled = CATALOGUE.map((message) => parser.compile(message));

    return () => {
      for (const message of compiled) message.resolve({ payload: PAYLOAD, locale: 'en' });
    };
  } },
  { name: 'resolve: five catalogue messages, each new to the parser', kind: 'time', run: () => () => {
    const parser = createParser();

    for (const message of MESSAGES) parser.resolve(message, { payload: PAYLOAD, locale: 'en' });
  } },
  { name: 'resolve: a stream of short messages, none seen before', kind: 'time', run: () => {
    const parser = createParser();
    let index = 0;

    // Shorter than 13 characters, so V8 lays each out in one piece,
    // and never the same twice, however many calls a sample takes.
    return () => parser.resolve(`{{a}}${(index++).toString(36)}`, { payload: { a: 'Ada' }, locale: 'en' });
  } },
  { name: 'resolve: a message of 12 000 characters, a new string each call', kind: 'time', run: () => {
    const parser = createParser();
    const text = ` world${'.'.repeat(11980)}`;
    let index = 0;

    // Joined anew each call, as a host that writes its text per call hands
    // it over, and shorter than the 16 384 characters past which V8
    // may hash a text by its length alone, so a lookup of it would show.
    return () => parser.resolve(`{{name}}${index++}${text}`, { payload: PAYLOAD, locale: 'en' });
  } },
  { name: 'resolve: 5 000 placeholders', kind: 'time', run: () => first(flat(5000), {}) },
  { name: 'resolve: nested 10 000 levels', kind: 'time', run: () => first(nested(10000), { v: 'a' }) },
  { name: 'resolve: a placeholder of 10 000 blanks', kind: 'time', run: () => first(`{{${' '.repeat(10000)}}}`, {}) },
  { name: 'resolve: a key with 10 000 blanks inside', kind: 'time', run: () => first(`{{a${' '.repeat(10000)}b}}`, {}) },
  { name: 'resolve: 1 000 placeholders that never close', kind: 'time', run: () => first('{{a; c:'.repeat(1000), {}) },
  { name: 'resolve: 5 000 options', kind: 'time', run: () => first(`{{a${'; a'.repeat(5000)}}}`, { a: 'a' }) },
  { name: 'resolve: an option key of 50 000 characters', kind: 'time', run: () => first(`{{a; ${'x'.repeat(50000)}:v}}`, { a: 'a' }) },
  { name: 'resolve: wrapper props of 5 000 entries', kind: 'time', run: () => {
    const payload = { v: { value: 'a', props: { test: Object.fromEntries(Array.from({ length: 5000 }, (_, index) => [`p${index}`, index])) } } };

    return () => resolveWrapped('{{v:test}}', { payload, props: { test: { q: 1 } }, locale: 'en' });
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
