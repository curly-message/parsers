import { describe, expect, it } from 'vitest';
import { plan } from '@curly-message/conformance';
import { adapter } from '../conformance/adapter';

// The conformance set, one test per case it plans for this implementation.
// The adapter claims every level, so the set leaves out only the case built
// from a bound on resolutions: this parser's bound counts kept reads, and a
// resolution a modifier asks for is a call of its own.
describe('conformance', () => {
  const { cases, skipped } = plan(adapter);

  it('plans every case the set ships', () => {
    expect(skipped.map(({ id }) => id)).toEqual(['limits-extensions/reentry-resolutions-over-limit']);
    expect(cases.length).toBeGreaterThan(0);
  });

  for (const planned of cases) {
    it(`${planned.id} (section ${planned.section})`, () => {
      expect(planned.execute()).toEqual({ ok: true });
    });
  }
});
