// Node's answers to the requests of the parity test, as the recorded host:
//
//   node tests/intl/golden.mjs tests/intl/requests.json > tests/intl/golden.json
//
// Each request is performed as section 11.2 delegates it, and its answer is
// recorded beside it: the text, the plural category, or the name of the
// error the facility raised. A request with no `timeZone` is formatted in
// UTC, the zone the Rust parser formats in where none is named. Text outside
// ASCII is written as JSON escapes, so the file shows every invisible space.
import { readFileSync } from 'node:fs';

process.env.TZ = 'UTC';

if (new Intl.DateTimeFormat().resolvedOptions().timeZone !== 'UTC') {
  throw new Error('the default time zone could not be set to UTC');
}

const answer = ({ locale, api, options, input }) => {
  try {
    switch (api) {
      case 'NumberFormat':
        return { text: new Intl.NumberFormat(locale, options).format(input) };
      case 'DateTimeFormat':
        return { text: new Intl.DateTimeFormat(locale, options).format(input) };
      case 'RelativeTimeFormat':
        return { text: new Intl.RelativeTimeFormat(locale, options).format(...input) };
      case 'PluralRules':
        return { category: new Intl.PluralRules(locale, options).select(input) };
      default:
        throw new Error(`no such facility: ${api}`);
    }
  } catch (error) {
    if (error instanceof RangeError || error instanceof TypeError) {
      return { error: error.name };
    }

    throw error;
  }
};

const requests = JSON.parse(readFileSync(process.argv[2], 'utf8'));
const golden = {
  node: process.versions.node,
  icu: process.versions.icu,
  answers: requests.map((request) => ({ ...request, answer: answer(request) })),
};
// JSON.stringify writes -0 as 0, and the sign is what some requests are about.
const MINUS_ZERO = '\u0000-0';
const ascii = JSON.stringify(golden, (_, value) => (Object.is(value, -0) ? MINUS_ZERO : value), 2)
  .replaceAll(JSON.stringify(MINUS_ZERO), '-0')
  .replace(
    /[^\x20-\x7e\n]/g,
    (character) => `\\u${character.charCodeAt(0).toString(16).padStart(4, '0')}`,
  );

process.stdout.write(`${ascii}\n`);
