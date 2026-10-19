# curly-message for Go

The Go implementation of the
[Curly Message Format](https://github.com/curly-message/spec),
`curly-message-3`.

```sh
go get github.com/curly-message/parsers/go
```

```go
import "github.com/curly-message/parsers/go/curly"
```

## Describing a message

`curly.CST(message)` returns the concrete syntax tree of
[CST.md](https://github.com/curly-message/spec/blob/main/CST.md): the
message, its placeholders and their parts, every escape sequence. Spans are
byte offsets into the message's UTF-8 (unit `utf-8`); a byte that starts no
valid UTF-8 sequence is one character, as `range` reads it.

`Node.AppendJSON` writes the interchange encoding of CST.md section 8 at any
depth of nesting. `json.Marshal` of a `Node` writes the same JSON, but with
`<`, `>` and `&` escaped as `\u003c`, `\u003e` and `\u0026`, as encoding/json
escapes them in what any `MarshalJSON` writes; a `json.Encoder` with
`SetEscapeHTML(false)` writes `AppendJSON`'s bytes. encoding/json refuses JSON
nested past 10 000 levels, so either fails on a tree nested past about 5 000
placeholders; `AppendJSON` does not.

## Development

Run from `go/`:

| Command | What it does |
|---------|--------------|
| `go build ./...` | build |
| `go vet ./...` | vet |
| `go test ./...` | the suite |
| `gofmt -l .` | prints every file the formatting contract rejects; `gofmt -w .` fixes them |
| `go run honnef.co/go/tools/cmd/staticcheck@v0.8.2 ./...` | lint |
| `go mod tidy -diff` | module hygiene |

Every `.go` file is ASCII: a character outside ASCII is written as an escape.

## License

[MIT](./LICENSE)
