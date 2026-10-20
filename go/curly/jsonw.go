package curly

import (
	"bytes"
	"encoding"
	"encoding/json"
	"math"
	"reflect"
	"slices"
	"unicode/utf8"
)

const hexDigits = "0123456789abcdef"

// appendJSONString writes s as a JSON string with HTML left unescaped: the
// quote, the backslash and control characters escaped as encoding/json
// escapes them, U+2028 and U+2029 escaped, as encoding/json escapes them
// for JSONP, and a byte that starts no valid sequence as the escape \ufffd.
// That is encoding/json's form up to Go 1.26; from 1.27 it writes the
// character itself. The parser keeps one form whatever the toolchain.
func appendJSONString(dst []byte, s string) []byte {
	dst = append(dst, '"')
	start := 0

	for i := 0; i < len(s); {
		if b := s[i]; b < utf8.RuneSelf {
			if b >= 0x20 && b != '"' && b != '\\' {
				i++

				continue
			}

			dst = append(dst, s[start:i]...)

			switch b {
			case '"', '\\':
				dst = append(dst, '\\', b)
			case '\b':
				dst = append(dst, '\\', 'b')
			case '\f':
				dst = append(dst, '\\', 'f')
			case '\n':
				dst = append(dst, '\\', 'n')
			case '\r':
				dst = append(dst, '\\', 'r')
			case '\t':
				dst = append(dst, '\\', 't')
			default:
				dst = append(dst, '\\', 'u', '0', '0', hexDigits[b>>4], hexDigits[b&0xf])
			}

			i++
			start = i

			continue
		}

		r, width := utf8.DecodeRuneInString(s[i:])

		if isInvalid(r, width) {
			dst = append(dst, s[start:i]...)
			dst = append(dst, `\ufffd`...)
			i++
			start = i

			continue
		}

		if r == '\u2028' || r == '\u2029' {
			dst = append(dst, s[start:i]...)
			dst = append(dst, '\\', 'u', '2', '0', '2', hexDigits[r&0xf])
			i += width
			start = i

			continue
		}

		i += width
	}

	dst = append(dst, s[start:]...)

	return append(dst, '"')
}

type jsonFrame struct {
	v      reflect.Value
	keys   []string
	i      int
	object bool
	wrote  bool
	id     pathID
}

// json writes a plain object or a plain array as encoding/json writes it
// with HTML escaping off: keys sorted by their bytes, strings escaped
// alike but for a byte that is not UTF-8, NaN and the infinities refused. The parser writes it rather than
// encoding/json because encoding/json recurses on the goroutine's stack and
// visits a shared value again at each reference: here one walk on a stack
// of its own both counts the nodes and writes them. A member holding
// Undefined is left out, still counted; an element holding it is null.
// Floats are written as section 3.3 writes them, which is encoding/json's
// form for every finite value but -0.
func (c *conversion) json(root reflect.Value) bool {
	var stack []jsonFrame

	path := map[pathID]bool{}

	if !c.jsonValue(root, &stack, path) {
		return false
	}

	for len(stack) > 0 {
		f := &stack[len(stack)-1]

		if f.object && f.i == len(f.keys) || !f.object && f.i == f.v.Len() {
			if f.object {
				c.buf = append(c.buf, '}')
			} else {
				c.buf = append(c.buf, ']')
			}

			delete(path, f.id)
			stack = stack[:len(stack)-1]

			continue
		}

		var member reflect.Value

		if f.object {
			key := f.keys[f.i]
			member = unwrap(f.v.MapIndex(reflect.ValueOf(key)))
			f.i++

			if isUndefined(member) {
				if !c.visit() {
					return false
				}

				continue
			}

			if f.wrote {
				c.buf = append(c.buf, ',')
			}

			f.wrote = true
			c.buf = appendJSONString(c.buf, key)
			c.buf = append(c.buf, ':')
		} else {
			member = unwrap(f.v.Index(f.i))

			if f.i > 0 {
				c.buf = append(c.buf, ',')
			}

			f.i++
		}

		if !c.jsonValue(member, &stack, path) {
			return false
		}
	}

	return true
}

// jsonValue writes one value, or opens the container it is and pushes it.
func (c *conversion) jsonValue(v reflect.Value, stack *[]jsonFrame, path map[pathID]bool) bool {
	if !c.visit() {
		return false
	}

	if !v.IsValid() || isUndefined(v) {
		c.buf = append(c.buf, "null"...)

		return true
	}

	t := v.Type()

	switch {
	case plainObject(t) || plainArray(t):
		if v.IsNil() {
			c.buf = append(c.buf, "null"...)

			return true
		}

		id := pathOf(v)

		if path[id] || c.nodes+v.Len() > MaxConversionNodes {
			return false
		}

		path[id] = true
		f := jsonFrame{v: v, object: t.Kind() == reflect.Map, id: id}

		if f.object {
			f.keys = make([]string, 0, v.Len())

			for it := v.MapRange(); it.Next(); {
				f.keys = append(f.keys, it.Key().String())
			}

			slices.Sort(f.keys)
			c.buf = append(c.buf, '{')
		} else {
			c.buf = append(c.buf, '[')
		}

		*stack = append(*stack, f)

		return true
	// A scalar type declared in no package, a predeclared one, has no
	// methods, and neither has its pointer.
	case t.NumMethod() == 0 && scalar(t.Kind()) && (t.PkgPath() == "" || !marshals(v)):
		switch t.Kind() {
		case reflect.String:
			c.buf = appendJSONString(c.buf, v.String())
		case reflect.Float32, reflect.Float64:
			if x := v.Float(); math.IsNaN(x) || math.IsInf(x, 0) {
				return false
			}

			fallthrough
		default:
			c.buf = appendScalar(c.buf, v)
		}

		return true
	}

	return c.hostJSON(v)
}

// hostJSON hands a member that is not plain data to encoding/json, which
// owns how such a value describes itself, once a walk of its own has
// counted what encoding/json will visit and measured how deep it nests. An
// element of a slice is handed over by its address, since encoding/json
// writing the slice reaches it addressable and calls a method of its
// pointer; a copy would be written without.
func (c *conversion) hostJSON(v reflect.Value) bool {
	if !v.CanInterface() {
		return false
	}

	if v.CanAddr() {
		v = v.Addr()
	}

	value := v.Interface()

	if !c.prewalk(reflect.ValueOf(value)) {
		return false
	}

	var out bytes.Buffer

	var err error

	completed := c.w.host(func() {
		enc := json.NewEncoder(&out)
		enc.SetEscapeHTML(false)
		err = enc.Encode(value)
	})

	if !completed || err != nil {
		return false
	}

	c.buf = append(c.buf, bytes.TrimSuffix(out.Bytes(), []byte{'\n'})...)

	return true
}

var (
	jsonMarshaler = reflect.TypeFor[json.Marshaler]()
	textMarshaler = reflect.TypeFor[encoding.TextMarshaler]()
)

// marshals reports whether encoding/json hands v to a method of its own,
// whose work is then the host's: by its type, or through its address where
// encoding/json can take one.
func marshals(v reflect.Value) bool {
	t := v.Type()

	if t.Implements(jsonMarshaler) || t.Implements(textMarshaler) {
		return true
	}

	if t.Kind() != reflect.Pointer && v.CanAddr() {
		p := reflect.PointerTo(t)

		return p.Implements(jsonMarshaler) || p.Implements(textMarshaler)
	}

	return false
}

// isBytes reports whether encoding/json writes a slice of type t in base64,
// as one value: its element is of a byte kind and marshals itself in no way,
// or encoding/json would call that element's method for each.
func isBytes(t reflect.Type) bool {
	if t.Elem().Kind() != reflect.Uint8 {
		return false
	}

	p := reflect.PointerTo(t.Elem())

	return !p.Implements(jsonMarshaler) && !p.Implements(textMarshaler)
}

type jsonField struct {
	index []int
}

// fieldsOf are the fields encoding/json may write of a struct type: the
// exported ones and those promoted from embedded structs, less those tagged
// "-". Counting a field it would leave out only spends budget early.
func (c *conversion) fieldsOf(t reflect.Type) []jsonField {
	if fields, ok := c.fields[t]; ok {
		return fields
	}

	var fields []jsonField

	for _, f := range reflect.VisibleFields(t) {
		tag := f.Tag.Get("json")

		if tag == "-" {
			continue
		}

		if f.Anonymous {
			ft := f.Type

			if ft.Kind() == reflect.Pointer {
				ft = ft.Elem()
			}

			if ft.Kind() == reflect.Struct && (tag == "" || tag[0] == ',') {
				continue
			}
		}

		if !f.IsExported() {
			continue
		}

		fields = append(fields, jsonField{f.Index})
	}

	if c.fields == nil {
		c.fields = map[reflect.Type][]jsonField{}
	}

	c.fields[t] = fields

	return fields
}

// prewalk counts the nodes encoding/json visits below v, charging them to
// the conversion, and refuses a value encoding/json cannot describe: one
// holding a cycle, a channel, a func or a complex number, or one nested
// deeper than hostDepth, which is what keeps encoding/json's recursion
// within a bounded stack.
func (c *conversion) prewalk(root reflect.Value) bool {
	type item struct {
		v     reflect.Value
		depth int
		exit  bool
		id    pathID
	}

	stack := []item{{v: root, depth: 1}}
	path := map[pathID]bool{}

	push := func(v reflect.Value, depth int) bool {
		if !c.visit() {
			return false
		}

		stack = append(stack, item{v: v, depth: depth})

		return true
	}

	for len(stack) > 0 {
		it := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		if it.exit {
			delete(path, it.id)

			continue
		}

		v := it.v

		if it.depth > hostDepth {
			return false
		}

		if !v.IsValid() || marshals(v) {
			continue
		}

		switch v.Kind() {
		case reflect.Interface:
			if !v.IsNil() {
				stack = append(stack, item{v: v.Elem(), depth: it.depth + 1})
			}
		case reflect.Pointer, reflect.Map, reflect.Slice:
			if v.IsNil() {
				continue
			}

			if v.Kind() == reflect.Slice && isBytes(v.Type()) {
				continue
			}

			id := pathOf(v)

			if path[id] {
				return false
			}

			path[id] = true
			stack = append(stack, item{exit: true, id: id})

			switch v.Kind() {
			case reflect.Pointer:
				stack = append(stack, item{v: v.Elem(), depth: it.depth + 1})
			case reflect.Map:
				switch v.Type().Key().Kind() {
				case reflect.String,
					reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
					reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
				default:
					if !v.Type().Key().Implements(textMarshaler) {
						return false
					}
				}

				for m := v.MapRange(); m.Next(); {
					if !push(m.Value(), it.depth+1) {
						return false
					}
				}
			default:
				for i := range v.Len() {
					if !push(v.Index(i), it.depth+1) {
						return false
					}
				}
			}
		case reflect.Array:
			for i := range v.Len() {
				if !push(v.Index(i), it.depth+1) {
					return false
				}
			}
		case reflect.Struct:
			for _, f := range c.fieldsOf(v.Type()) {
				field, err := v.FieldByIndexErr(f.index)

				if err != nil {
					continue
				}

				if !push(field, it.depth+1) {
					return false
				}
			}
		case reflect.Chan, reflect.Func, reflect.Complex64, reflect.Complex128, reflect.UnsafePointer:
			return false
		}
	}

	return true
}
