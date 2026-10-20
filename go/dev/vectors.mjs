// Writes curly/testdata/floats.json: numbers with the text ECMAScript's
// Number::toString gives them, which section 3.3 of the Go design takes as
// the ordinary conversion of a float. Run from go/ with `node dev/vectors.mjs`
// and commit what it writes; the Go suite reads it and never runs Node.
import { writeFileSync } from 'node:fs';

const bits64 = (x) => {
  const view = new DataView(new ArrayBuffer(8));

  view.setFloat64(0, x);

  return view.getBigUint64(0).toString(16).padStart(16, '0');
};

const bits32 = (x) => {
  const view = new DataView(new ArrayBuffer(4));

  view.setFloat32(0, x);

  return view.getUint32(0).toString(16).padStart(8, '0');
};

// The shortest digits that read back as the same float32, which is what Go
// writes a float32 with: the fewest significant digits whose nearest float32
// is the value itself.
const shortest32 = (x) => {
  if (!Number.isFinite(x) || x === 0) return String(x);

  for (let digits = 1; digits <= 9; digits += 1) {
    const candidate = Number(x.toPrecision(digits));

    if (Math.fround(candidate) === x) return String(candidate);
  }

  throw new RangeError(`no shortest form for ${x}`);
};

const doubles = [
  0, -0, 1, -1, 0.1, 0.2, 0.30000000000000004, 1 / 3, 2 / 3, 42, 100, 1e6, 2.5e6, 1e-5, 1e-6, 1e-7,
  0.000001, 0.0000015, 1e20, 1e21, 1.5e21, 123456789012345680000, 1e100, 1.7976931348623157e308,
  5e-324, 2.2250738585072014e-308, 2.225073858507201e-308, 9007199254740992, 9007199254740993,
  -1e21, -1e-7, 1.25, 3.14159, 1e300, 4.35, 0.5, 1234.5678, Number.EPSILON, 1e-300, 123e-20,
  NaN, Infinity, -Infinity,
];

// A spread of values across the exponent range, so the thresholds are met
// from both sides and at every digit count.
for (let exponent = -30; exponent <= 30; exponent += 1) {
  for (const mantissa of [1, 1.5, 9.999999999999999, 1.2345678901234567]) doubles.push(mantissa * 10 ** exponent);
}

let seed = 1;
const random = () => {
  seed = (seed * 48271) % 2147483647;

  return seed / 2147483647;
};

for (let i = 0; i < 200; i += 1) doubles.push((random() - 0.5) * 10 ** Math.floor(random() * 60 - 30));

const singles = [0, 1, 0.1, 1 / 3, 16777216, 16777217, 3.4028234663852886e38, 1.401298464324817e-45, 1e-7, 1e21, 1e-6, 123456.789, 0.3];

for (let exponent = -40; exponent <= 38; exponent += 3) singles.push(1.2345678 * 10 ** exponent);

for (let i = 0; i < 100; i += 1) singles.push((random() - 0.5) * 10 ** Math.floor(random() * 40 - 20));

const vectors = {
  float64: doubles.map((x) => [bits64(x), String(x)]),
  float32: singles.map((x) => Math.fround(x)).map((x) => [bits32(x), shortest32(x)]),
};

// One vector a line, so a regenerated file diffs by the vectors it changed.
const lines = (pairs) => pairs.map((pair) => `  ${JSON.stringify(pair)}`).join(',\n');
const text = `{\n "float64": [\n${lines(vectors.float64)}\n ],\n "float32": [\n${lines(vectors.float32)}\n ]\n}\n`;

writeFileSync(new URL('../curly/testdata/floats.json', import.meta.url), text);
