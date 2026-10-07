import ts from 'typescript';
import { describe, expect, it } from 'vitest';

// TypeScript names files with forward slashes on every platform.
const root = ts.sys.getCurrentDirectory().replaceAll('\\', '/');
// Inside the package, so the import below reaches the declarations the build
// ships through the package's own `exports`, as a consumer's import does. It
// is served from memory and never written.
const file = `${root}/probe.ts`;

const options: ts.CompilerOptions = {
  module: ts.ModuleKind.ESNext,
  moduleResolution: ts.ModuleResolutionKind.Bundler,
  noEmit: true,
  skipLibCheck: true,
  strict: true,
  target: ts.ScriptTarget.ES2022,
  types: [],
};

// The instantiations checking `keys` resolve calls costs, each naming one key
// of a payload type of `keys` keys and passing a payload literal of its own,
// as a call site writes one: a literal is a type of its own, so no call
// reuses what the checker worked out for another.
const instantiations = (keys: number) => {
  const names = Array.from({ length: keys }, (_, index) => `key${index}`);
  const payload = `{ ${names.map((name) => `${name}: 'value'`).join(', ')} }`;
  const text = [
    "import { createParser } from '@curly-message/parser';",
    `type Payload = { ${names.map((name) => `${name}: string;`).join(' ')} };`,
    'const { resolve } = createParser<Payload>();',
    ...names.map((name) => `resolve('{{${name}}}', { payload: ${payload} });`),
  ].join('\n');

  const host = ts.createCompilerHost(options);
  const fileExists = host.fileExists.bind(host);
  const readFile = host.readFile.bind(host);
  const getSourceFile = host.getSourceFile.bind(host);
  host.fileExists = (name) => name === file || fileExists(name);
  host.readFile = (name) => (name === file ? text : readFile(name));
  host.getSourceFile = (name, version) => (name === file ? ts.createSourceFile(name, text, version) : getSourceFile(name, version));

  const program = ts.createProgram([file], options, host);
  const diagnostics = ts.getPreEmitDiagnostics(program).map(({ messageText }) => ts.flattenDiagnosticMessageText(messageText, '\n'));

  expect(diagnostics).toEqual([]);

  return program.getInstantiationCount();
};

// Counted rather than timed: a count is the same on every machine and every
// run, so the bound can be tight.
describe('checker cost', () => {
  it('costs a call the same whatever the size of its payload', () => {
    expect(instantiations(40)).toBeLessThanOrEqual(2 * instantiations(20));
  });
});
