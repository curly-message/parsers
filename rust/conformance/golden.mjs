// Node's answers to the requests the conformance set states, as the recorded
// host of the runner's text reading:
//
//   cargo run -p curly-message-conformance --bin requests \
//     | node conformance/golden.mjs > conformance/set/golden-4.1.0.json
//
// Each request is performed as the JavaScript runner's `format()` performs
// it, and its answer is recorded beside it: the text, the plural category, or
// the name of the error the facility raised. The JavaScript runner formats a
// request with no `timeZone` in the zone of the machine it runs on, which is
// UTC where the set is tested, so this does too. Text outside ASCII is written
// as JSON escapes, so the file shows every invisible space.
import { readFileSync } from 'node:fs';

process.env.TZ = 'UTC';

if (new Intl.DateTimeFormat().resolvedOptions().timeZone !== 'UTC') {
  throw new Error('the default time zone could not be set to UTC');
}

const answer = ({ locale, api, options, input }) => {
  try {
    switch (api) {
      case 'PluralRules':
        return { category: new Intl.PluralRules(locale, options).select(input) };
      case 'NumberFormat':
        return { text: new Intl.NumberFormat(locale, options).format(input) };
      case 'DateTimeFormat':
        return { text: new Intl.DateTimeFormat(locale, options).format(input) };
      case 'RelativeTimeFormat': {
        const [value, unit] = input;

        return { text: new Intl.RelativeTimeFormat(locale, options).format(value, unit) };
      }
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

const requests = JSON.parse(readFileSync(0, 'utf8'));
const golden = {
  node: process.versions.node,
  icu: process.versions.icu,
  cldr: process.versions.cldr,
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
