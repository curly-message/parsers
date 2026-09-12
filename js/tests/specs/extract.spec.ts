import { describe, expect, it } from 'vitest';
import { createExtractor, Parser } from '../../src';
import { LINE_TERM } from '../../src/utils';

const params = (message: any, options?: Parameters<Parser.ExtractorFactory>[0]) => createExtractor(options)(message);

const names = (message: string) => params(message).map(({ name }) => name);

const kinds = (message: string) => params(message).map(({ kind }) => kind);

describe('the parameters a message names', () => {
  it('names each key once, in the order the message first names it', () => {
    expect(names('{{b}} {{a}} {{b}}')).toEqual(['b', 'a']);
  });

  it('names nothing where the message carries no placeholder', () => {
    expect(params('Hello.')).toEqual([]);
  });

  it('names nothing where a placeholder names no key', () => {
    expect(params('{{:number}} {{ }} {{;a:1}}')).toEqual([]);
  });

  it('names a key the way the payload is keyed by it', () => {
    expect(names('{{a\\:b}} {{ spaced }} {{a\\ b}}')).toEqual(['a:b', 'spaced', 'a b']);
  });

  it('names the reserved `default` key like any other', () => {
    expect(params('{{default}}')).toEqual([{ name: 'default', kind: 'unknown', optional: false }]);
  });

  it('names a key spelled like a prototype member', () => {
    expect(names('{{toString}} {{__proto__}} {{constructor}}')).toEqual(['toString', '__proto__', 'constructor']);
  });

  it('names nothing for a message that is not text', () => {
    expect(params(undefined)).toEqual([]);
    expect(params(null)).toEqual([]);
    expect(params(42)).toEqual([]);
    expect(params({ a: '{{name}}' })).toEqual([]);
  });

  it('names nothing for braces no placeholder closes', () => {
    expect(params('{{a')).toEqual([]);
    expect(params('{a}')).toEqual([]);
    expect(params('\\{{a}}')).toEqual([]);
  });

  it('names nothing for a placeholder carrying a line terminator', () => {
    LINE_TERM.forEach((terminator) => {
      expect(params(`{{a${terminator}b}}`)).toEqual([]);
    });
  });

  it('names what the surviving placeholder names where one holds another', () => {
    expect(names('{{n:eq; 1:you have {{x}}}}')).toEqual(['x']);
  });
});

describe('what a parameter accepts', () => {
  it('narrows nothing where the modifier reads the value as text', () => {
    expect(kinds('{{a}} {{b:eq; 1:one}} {{c:ne; 1:one}}')).toEqual(['unknown', 'unknown', 'unknown']);
  });

  it('narrows a numeric comparison to a number', () => {
    expect(kinds('{{a:lt; 5:few}} {{b:gt; 5:many}}')).toEqual(['number', 'number']);
  });

  it('narrows an inequality that also matches text to either', () => {
    expect(kinds('{{a:lte; 5:few}} {{b:gte; 5:many}}')).toEqual([['number', 'string'], ['number', 'string']]);
  });

  it('narrows a formatting modifier to what it formats', () => {
    expect(kinds('{{a:number}} {{b:currency}} {{c:ago}} {{d:date}}')).toEqual(['number', 'number', 'number', ['date', 'string']]);
  });

  it('narrows nothing for a modifier nobody registered', () => {
    expect(kinds('{{a:nosuch}} {{b:toString}} {{c:constructor}}')).toEqual(['unknown', 'unknown', 'unknown']);
  });

  it('says what every placeholder naming the key says together', () => {
    expect(kinds('{{a}} {{a:number}}')).toEqual(['number']);
    expect(kinds('{{a:number}} {{a:date}}')).toEqual([['number', 'date', 'string']]);
    expect(kinds('{{a:number}} {{a:currency}}')).toEqual(['number']);
  });
});

describe('the values a message names', () => {
  it('lists the option keys an equality selects among', () => {
    expect(params('{{a:eq; yes:Y; no:N}}')).toEqual([{ name: 'a', kind: 'unknown', values: ['yes', 'no'], optional: false }]);
  });

  it('lists them for a placeholder that names no modifier', () => {
    expect(params('{{a; yes:Y; no:N}}')[0]?.values).toEqual(['yes', 'no']);
  });

  it('reads an option that names no value as its own key', () => {
    expect(params('{{a; yes; no}}')[0]?.values).toEqual(['yes', 'no']);
  });

  it('lists a key the way the message compares it', () => {
    expect(params('{{a; y\\:z:Y; \\ pad\\ :P}}')[0]?.values).toEqual(['y:z', ' pad ']);
  });

  it('never lists the inline default', () => {
    expect(params('{{a; yes:Y; default:N}}')[0]?.values).toEqual(['yes']);
  });

  it('lists nothing where the keys are not values of the parameter', () => {
    expect(params('{{a:ne; yes:Y}}')[0]).not.toHaveProperty('values');
    expect(params('{{a:lt; 5:few}}')[0]).not.toHaveProperty('values');
    expect(params('{{a:gte; 5:many}}')[0]).not.toHaveProperty('values');
    expect(params('{{a:number; 5:five}}')[0]).not.toHaveProperty('values');
    expect(params('{{a:nosuch; 5:five}}')[0]).not.toHaveProperty('values');
  });

  it('lists what every placeholder naming the key lists together', () => {
    expect(params('{{a; yes:Y}} {{a:eq; no:N; yes:Y}}')[0]?.values).toEqual(['yes', 'no']);
  });
});

describe('whether the message states a fallback', () => {
  it('reports a parameter the message expects', () => {
    expect(params('{{a}}')[0]?.optional).toBe(false);
  });

  it('reports a parameter the placeholder declares a default for', () => {
    expect(params('{{a; default:none}}')[0]?.optional).toBe(true);
    expect(params('{{a; default:}}')[0]?.optional).toBe(true);
    expect(params('{{a; default}}')[0]?.optional).toBe(true);
  });

  it('reports it where any placeholder naming the key declares one', () => {
    expect(params('{{a}} {{a; default:none}}')[0]?.optional).toBe(true);
  });

  it('reads only the lowercase spelling as a default', () => {
    expect(params('{{a; Default:none}}')[0]?.optional).toBe(false);
  });
});

describe('a host that registered its own modifiers', () => {
  const custom = { customModifiers: { shout: ({ value }: { value: string }) => value.toUpperCase() } };

  it('narrows nothing for a name the host registered', () => {
    expect(params('{{a:shout}}', custom)[0]?.kind).toBe('unknown');
    expect(params('{{a:shout; x:1}}', custom)[0]).not.toHaveProperty('values');
  });

  it('stops reading a built-in name the host replaced', () => {
    const replaced = { customModifiers: { number: ({ value }: { value: string }) => value } };

    expect(params('{{a:number}}', replaced)[0]?.kind).toBe('unknown');
  });

  it('stops listing option keys where the host replaced the equality', () => {
    const replaced = { customModifiers: { eq: ({ value }: { value: string }) => value } };

    expect(params('{{a; yes:Y}}', replaced)[0]).not.toHaveProperty('values');
    expect(params('{{a:eq; yes:Y}}', replaced)[0]).not.toHaveProperty('values');
  });

  it('reads a registry entry that is not a modifier as registering nothing', () => {
    const broken = { customModifiers: { number: 'not a modifier' } } as any;

    expect(params('{{a:number}}', broken)[0]?.kind).toBe('number');
  });

  it('reads no options at all where the host registered none', () => {
    expect(params('{{a:number}}', {})[0]?.kind).toBe('number');
    expect(params('{{a:number}}', undefined)[0]?.kind).toBe('number');
  });
});
