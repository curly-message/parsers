// Package curly implements the Curly Message Format, curly-message-3
// (https://github.com/curly-message/spec).
//
// New returns a Parser, and Parser.Resolve resolves a message against a
// payload at the Core level of the format: substitution, the fallback chain,
// value wrappers and the comparison modifiers, and at the Extensions level:
// the modifiers a host registers through Options.CustomModifiers. A payload
// value becomes text by its own type: a plain object or array as JSON, a
// number as ECMAScript writes it, anything else as fmt prints it.
//
// CST describes a message as it is written: its placeholders, what each is
// made of, and where every escape sequence falls, as the concrete syntax tree
// of CST.md. Spans are byte offsets into the message's UTF-8 (CST.md section
// 4, unit utf-8), and a byte that starts no valid UTF-8 sequence is one
// character standing for itself, as range reads it.
//
// A resolution recurses over a message at most MaxNesting levels, one for
// each option value or inline default a placeholder holds, and hands
// encoding/json, which recurses, only a value at most 64 levels deep,
// counted over every field it may write. Everything else walks a stack of
// its own: the scan, the tree and its encoding, the resolution past
// MaxNesting and the conversion of a value. So a message or a value nested
// to any depth costs the goroutine's stack no more than those two bounds
// allow. Built on encoding/json/v2, from Go 1.27, encoding/json also
// re-formats what a MarshalJSON method writes, recursively and up to its own
// limit of 10 000 levels, once the method has returned: that cost is
// encoding/json's bound, not the parser's.
package curly
