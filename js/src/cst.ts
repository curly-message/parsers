import type { Cst } from './types';
import { cancels, escapeEnd, scanner, separated, trimmed, unesc } from './utils';
import type { Scanner } from './utils';

/**
 * Text with every escape sequence in it marked. Section 7's rule is uniform
 * over the whole message string, so the same scan reads a message, a name and
 * an option value: a backslash consumes the character after it wherever it
 * appears. A backslash with nothing left to consume denotes itself and is
 * text, which is what a trailing one in a message is.
 */
const spelled = (message: string, from: number, to: number): (Cst.Text | Cst.Escape)[] => {
  const nodes: (Cst.Text | Cst.Escape)[] = [];
  let start = from;

  for (let index = from; index < to; index += 1) {
    if (message[index] !== '\\' || index + 1 >= to) continue;

    if (index > start) nodes.push({ type: 'text', start, end: index });

    const end = escapeEnd(message, index, to);

    nodes.push({ type: 'escape', start: index, end, cancels: cancels(message.slice(index + 1, end)) });
    index = end - 1;
    start = end;
  }

  if (to > start) nodes.push({ type: 'text', start, end: to });

  return nodes;
};

/**
 * What a span of the message itself holds: its text, its escape sequences and
 * the placeholders it writes. The message is one such span and so is each
 * option value, which is why a placeholder is described inside one and inside
 * nothing else (section 6, note 10).
 */
const contents = (message: string, scan: Scanner, from: number, to: number): (Cst.Text | Cst.Escape | Cst.Placeholder)[] => {
  const nodes: (Cst.Text | Cst.Escape | Cst.Placeholder)[] = [];
  // A span holds as many parts as it holds characters, which is more than a
  // call can be spread over: they are appended one at a time.
  const append = (added: readonly (Cst.Text | Cst.Escape)[]) => { for (const node of added) nodes.push(node); };
  let at = from;

  for (let match = scan.next(at, to); match; match = scan.next(at, to)) {
    const [open, close] = match;

    append(spelled(message, at, open));
    nodes.push(placeholder(message, scan, open, close));
    at = close;
  }

  append(spelled(message, at, to));

  return nodes;
};

// A name and the padding it is read without. The padding is nodes of its own
// rather than width the name quietly covers, so the parts still tile the
// message: what is dropped is said, not omitted.
const named = (message: string, type: Cst.Name['type'], from: number, to: number): (Cst.Space | Cst.Name)[] => {
  const [start, end] = trimmed(message, from, to);

  return [
    ...(start > from ? [{ type: 'space' as const, start: from, end: start }] : []),
    { type, start, end, name: unesc(message.slice(start, end)), nodes: spelled(message, start, end) },
    ...(to > end ? [{ type: 'space' as const, start: end, end: to }] : []),
  ];
};

// An option value and the padding it is read without. It answers to nobody, so
// it carries the subtree the message spells and no name to compare.
const valued = (message: string, scan: Scanner, from: number, to: number): (Cst.Space | Cst.OptionValue)[] => {
  const [start, end] = trimmed(message, from, to);

  return [
    ...(start > from ? [{ type: 'space' as const, start: from, end: start }] : []),
    { type: 'option-value' as const, start, end, nodes: contents(message, scan, start, end) },
    ...(to > end ? [{ type: 'space' as const, start: end, end: to }] : []),
  ];
};

const placeholder = (message: string, scan: Scanner, open: number, close: number): Cst.Placeholder => {
  const nodes: Cst.Placeholder['nodes'] = [{ type: 'open', start: open, end: open + 2 }];
  // A `;` a nested placeholder encloses divides that placeholder and not this
  // one (section 6, note 5), so the segments are cut around what each option
  // value holds.
  const [declaration, ...options] = separated(message, open + 2, close - 2, ';', scan.end);

  // The selector's colon is the first one no escape sequence claims, and
  // everything after it is the modifier name — a second colon is part of the
  // name rather than a second separator (section 6, note 3).
  const [key, ...modifier] = separated(message, declaration[0], declaration[1], ':');

  nodes.push(...named(message, 'key', key[0], key[1]));

  if (modifier.length) {
    nodes.push({ type: 'separator', start: key[1], end: key[1] + 1 });
    nodes.push(...named(message, 'modifier', key[1] + 1, declaration[1]));
  }

  for (const [from, to] of options) {
    // The `;` that opened this segment sits immediately before it.
    nodes.push({ type: 'separator', start: from - 1, end: from });

    const [optionKey, ...value] = separated(message, from, to, ':', scan.end);

    nodes.push(...named(message, 'option-key', optionKey[0], optionKey[1]));

    // A segment that states no value stands for its own key (section 9.4), so
    // there is no value to describe: the key is where the message writes it.
    if (value.length) {
      nodes.push({ type: 'separator', start: optionKey[1], end: optionKey[1] + 1 });
      nodes.push(...valued(message, scan, optionKey[1] + 1, to));
    }
  }

  nodes.push({ type: 'close', start: close - 2, end: close });

  return { type: 'placeholder', start: open, end: close, nodes };
};

/**
 * Describes a message: where its placeholders are, what each is made of, and
 * where every escape sequence falls.
 *
 * The placeholders are the ones resolution finds, because the same scan finds
 * them: a placeholder written in an option value is described there, and a
 * construct that derives nothing is text — what an editor shows is what the
 * message does.
 */
export const cst: Cst.Parse = (message) => {
  if (typeof message !== 'string') return { type: 'message', start: 0, end: 0, nodes: [] };

  return { type: 'message', start: 0, end: message.length, nodes: contents(message, scanner(message), 0, message.length) };
};
