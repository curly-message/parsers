// Package curly implements the Curly Message Format, curly-message-3
// (https://github.com/curly-message/spec).
//
// CST describes a message as it is written: its placeholders, what each is
// made of, and where every escape sequence falls, as the concrete syntax tree
// of CST.md. Spans are byte offsets into the message's UTF-8 (CST.md section
// 4, unit utf-8), and a byte that starts no valid UTF-8 sequence is one
// character standing for itself, as range reads it.
//
// Nothing in the package recurses over a message: the scan, the tree and its
// encoding each walk a stack of their own, so a message nested to any depth
// costs the goroutine's stack nothing more than a flat one.
package curly
