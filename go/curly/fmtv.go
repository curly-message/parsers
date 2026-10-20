package curly

import (
	"cmp"
	"fmt"
	"reflect"
	"strconv"
)

// state is the fmt.State a Formatter is handed: a buffer of its own, no
// flags, no width and no precision, which is what fmt hands one for %v.
type state struct {
	buf []byte
}

func (s *state) Write(b []byte) (int, error) {
	s.buf = append(s.buf, b...)

	return len(b), nil
}

func (s *state) Width() (int, bool)     { return 0, false }
func (s *state) Precision() (int, bool) { return 0, false }
func (s *state) Flag(int) bool          { return false }

type printFrame struct {
	v      reflect.Value
	kind   reflect.Kind
	i      int
	depth  int
	sorted []entry
	id     pathID
	onPath bool
}

// print writes v as fmt.Sprint writes it, without fmt's recursion: an
// iterative port of fmt's printValue for the verb v and of fmtsort's key
// order, so a value nested to any depth costs the goroutine's stack nothing
// and a container holding itself is a cycle rather than a fatal overflow.
// Where fmt would hand a value to its Format, Error or String method, at any
// depth it can, the parser calls the method itself, contained: a method that
// panics leaves the whole value undescribable, rather than putting the
// panic's text in the output as fmt does, unless it panicked on a nil
// pointer receiver, which writes <nil> as fmt does.
func (c *conversion) print(arg any) bool {
	if !c.visit() {
		return false
	}

	if arg == nil {
		c.buf = append(c.buf, "<nil>"...)

		return true
	}

	var stack []printFrame

	path := map[pathID]bool{}

	if rv, ok := arg.(reflect.Value); ok {
		if rv.IsValid() && rv.CanInterface() {
			handled, ok := c.methods(rv.Interface())

			if handled {
				return ok
			}
		}

		if !c.printValue(rv, 0, &stack, path) {
			return false
		}
	} else {
		handled, ok := c.methods(arg)

		if handled {
			return ok
		}

		if !c.printValue(reflect.ValueOf(arg), 0, &stack, path) {
			return false
		}
	}

	for len(stack) > 0 {
		f := &stack[len(stack)-1]

		var child reflect.Value

		depth := f.depth + 1

		switch f.kind {
		case reflect.Map:
			if f.i == 2*len(f.sorted) {
				c.buf = append(c.buf, ']')
				c.pop(&stack, path)

				continue
			}

			switch {
			case f.i%2 == 1:
				c.buf = append(c.buf, ':')
				child = f.sorted[f.i/2].value
			case f.i > 0:
				c.buf = append(c.buf, ' ')

				fallthrough
			default:
				child = f.sorted[f.i/2].key
			}
		case reflect.Struct:
			if f.i == f.v.NumField() {
				c.buf = append(c.buf, '}')
				c.pop(&stack, path)

				continue
			}

			if f.i > 0 {
				c.buf = append(c.buf, ' ')
			}

			child = f.v.Field(f.i)

			if child.Kind() == reflect.Interface && !child.IsNil() {
				child = child.Elem()
			}
		default:
			if f.i == f.v.Len() {
				c.buf = append(c.buf, ']')
				c.pop(&stack, path)

				continue
			}

			if f.i > 0 {
				c.buf = append(c.buf, ' ')
			}

			child = f.v.Index(f.i)
		}

		f.i++

		if !c.visit() {
			return false
		}

		if !c.printValue(child, depth, &stack, path) {
			return false
		}
	}

	return true
}

func (c *conversion) pop(stack *[]printFrame, path map[pathID]bool) {
	f := (*stack)[len(*stack)-1]

	if f.onPath {
		delete(path, f.id)
	}

	*stack = (*stack)[:len(*stack)-1]
}

// methods is fmt's handleMethods for %v: a Formatter, then an error, then a
// Stringer. It reports whether a method handled the value, and whether the
// value is still describable.
func (c *conversion) methods(arg any) (handled, ok bool) {
	var text string

	var completed bool

	switch m := arg.(type) {
	case fmt.Formatter:
		s := &state{}
		completed = c.w.host(func() { m.Format(s, 'v') })
		text = string(s.buf)
	case error:
		completed = c.w.host(func() { text = m.Error() })
	case fmt.Stringer:
		completed = c.w.host(func() { text = m.String() })
	default:
		return false, true
	}

	c.buf = append(c.buf, text...)

	if !completed {
		if c.w.closed {
			return true, false
		}

		if v := reflect.ValueOf(arg); v.Kind() != reflect.Pointer || !v.IsNil() {
			return true, false
		}

		c.buf = append(c.buf, "<nil>"...)
	}

	return true, true
}

// printValue writes one value, or opens the container it is and pushes it,
// following an interface or a pointer at the top level in place.
func (c *conversion) printValue(v reflect.Value, depth int, stack *[]printFrame, path map[pathID]bool) bool {
	for {
		if depth > 0 && v.IsValid() && v.CanInterface() {
			if handled, ok := c.methods(v.Interface()); handled {
				return ok
			}
		}

		switch v.Kind() {
		case reflect.Invalid:
			if depth == 0 {
				c.buf = append(c.buf, "<invalid reflect.Value>"...)
			} else {
				c.buf = append(c.buf, "<nil>"...)
			}
		case reflect.Bool:
			c.buf = strconv.AppendBool(c.buf, v.Bool())
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			c.buf = strconv.AppendInt(c.buf, v.Int(), 10)
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			c.buf = strconv.AppendUint(c.buf, v.Uint(), 10)
		case reflect.Float32:
			c.buf = appendFmtFloat(c.buf, v.Float(), 32, false)
		case reflect.Float64:
			c.buf = appendFmtFloat(c.buf, v.Float(), 64, false)
		case reflect.Complex64, reflect.Complex128:
			bits := 64

			if v.Kind() == reflect.Complex64 {
				bits = 32
			}

			z := v.Complex()
			c.buf = append(c.buf, '(')
			c.buf = appendFmtFloat(c.buf, real(z), bits, false)
			c.buf = appendFmtFloat(c.buf, imag(z), bits, true)
			c.buf = append(c.buf, 'i', ')')
		case reflect.String:
			if !c.fits(len(v.String())) {
				return false
			}

			c.buf = append(c.buf, v.String()...)
		case reflect.Map:
			id := pathOf(v)

			if path[id] || c.nodes+2*v.Len() > c.limit {
				return false
			}

			sorted, ok := c.sortedKeys(v)

			if !ok {
				return false
			}

			c.buf = append(c.buf, "map["...)
			path[id] = true
			*stack = append(*stack, printFrame{v: v, kind: reflect.Map, depth: depth, sorted: sorted, id: id, onPath: true})
		case reflect.Struct:
			c.buf = append(c.buf, '{')
			*stack = append(*stack, printFrame{v: v, kind: reflect.Struct, depth: depth})
		case reflect.Interface:
			elem := v.Elem()

			if !elem.IsValid() {
				c.buf = append(c.buf, "<nil>"...)

				return true
			}

			v, depth = elem, depth+1

			if !c.visit() {
				return false
			}

			continue
		case reflect.Array, reflect.Slice:
			f := printFrame{v: v, kind: v.Kind(), depth: depth}

			if c.nodes+v.Len() > c.limit {
				return false
			}

			if v.Kind() == reflect.Slice {
				f.id, f.onPath = pathOf(v), true

				if path[f.id] {
					return false
				}

				path[f.id] = true
			}

			c.buf = append(c.buf, '[')
			*stack = append(*stack, f)
		case reflect.Pointer:
			if depth == 0 && !v.IsNil() {
				switch elem := v.Elem(); elem.Kind() {
				case reflect.Array, reflect.Slice, reflect.Struct, reflect.Map:
					c.buf = append(c.buf, '&')
					v, depth = elem, depth+1

					if !c.visit() {
						return false
					}

					continue
				}
			}

			fallthrough
		case reflect.Chan, reflect.Func, reflect.UnsafePointer:
			if p := uintptr(v.UnsafePointer()); p == 0 {
				c.buf = append(c.buf, "<nil>"...)
			} else {
				c.buf = append(c.buf, '0', 'x')
				c.buf = strconv.AppendUint(c.buf, uint64(p), 16)
			}
		}

		return true
	}
}

// appendFmtFloat writes a float as fmt's %v does, %g at the shortest
// precision, with the sign fmt writes on an infinity, and on every part
// where signed is set, as fmt writes the imaginary part of a complex.
func appendFmtFloat(dst []byte, x float64, bits int, signed bool) []byte {
	text := strconv.AppendFloat(nil, x, 'g', -1, bits)

	switch {
	case text[0] == 'N':
		if signed {
			dst = append(dst, '+')
		}
	case text[0] == '+':
	case text[0] != '-' && (signed || text[0] == 'I'):
		dst = append(dst, '+')
	}

	return append(dst, text...)
}

type entry struct {
	key, value reflect.Value
}

// sortedKeys is a map's entries in fmtsort's order, each value read with
// its key, since a key holding NaN finds nothing when it is looked up. The
// keys of a map of two keys or more are counted before they are sorted,
// against a budget of their own (printLimit), so that the sort, which reads
// at most that count in each of its passes, spends none of what the
// conversion may visit. A map is sorted, and counted, at each reference it
// is reached through.
func (c *conversion) sortedKeys(m reflect.Value) ([]entry, bool) {
	entries := make([]entry, 0, m.Len())

	for it := m.MapRange(); it.Next(); {
		entries = append(entries, entry{it.Key(), it.Value()})
	}

	if len(entries) < 2 {
		return entries, true
	}

	for _, e := range entries {
		if !c.keys.count(e.key, c.limit) {
			return nil, false
		}
	}

	c.keys.sort(entries)

	return entries, true
}

// keyOrder compares map keys on one stack for every key of a map, a frame
// for each level a key nests, since a key can nest structs, arrays and
// interfaces to any depth. nodes is what counting keys has counted, and
// reads the pairs of nodes comparing them has read.
type keyOrder struct {
	stack []keyFrame
	nodes int
	reads int
}

// keyFrame is a key, or a pair of keys to compare, or, where n is set,
// structs or arrays whose members from i on are still to be counted or
// compared.
type keyFrame struct {
	a, b reflect.Value
	i, n int
}

// count adds to nodes what comparing a key with one alike reads, a node for
// the key and for each value inside it, and a node for each byte of a
// string, which a comparison reads as well, and reports whether nodes stays
// within limit. Comparing two keys reads no pair past what either holds, so
// each pass of the sort reads at most what its keys count.
func (k *keyOrder) count(v reflect.Value, limit int) bool {
	stack := append(k.stack[:0], keyFrame{a: v, n: -1})

	defer func() { k.stack = stack[:0] }()

	for len(stack) > 0 {
		f := &stack[len(stack)-1]

		if f.n >= 0 {
			if f.i == f.n {
				stack = stack[:len(stack)-1]

				continue
			}

			x := keyMember(f.a, f.i)
			f.i++
			stack = append(stack, keyFrame{a: x, n: -1})

			continue
		}

		v := f.a
		stack = stack[:len(stack)-1]
		k.nodes++

		switch v.Kind() {
		case reflect.String:
			k.nodes += v.Len()
		case reflect.Struct:
			stack = append(stack, keyFrame{a: v, n: v.NumField()})
		case reflect.Array:
			stack = append(stack, keyFrame{a: v, n: v.Len()})
		case reflect.Interface:
			if !v.IsNil() {
				stack = append(stack, keyFrame{a: v.Elem(), n: -1})
			}
		}

		if k.nodes > limit {
			return false
		}
	}

	return true
}

// keyMember is a struct's field or an array's element.
func keyMember(v reflect.Value, i int) reflect.Value {
	if v.Kind() == reflect.Struct {
		return v.Field(i)
	}

	return v.Index(i)
}

// sort is a stable merge sort, as fmtsort's is stable: over n keys it makes
// ceil(log2 n) passes, each of which compares each key with at most one other
// as it places it.
func (k *keyOrder) sort(entries []entry) {
	src, dst := entries, make([]entry, len(entries))

	for width := 1; width < len(src); width *= 2 {
		for lo := 0; lo < len(src); lo += 2 * width {
			mid, hi := min(lo+width, len(src)), min(lo+2*width, len(src))
			i, j, o := lo, mid, lo

			for ; i < mid && j < hi; o++ {
				if k.compare(src[j].key, src[i].key) < 0 {
					dst[o], j = src[j], j+1
				} else {
					dst[o], i = src[i], i+1
				}
			}

			o += copy(dst[o:], src[i:mid])
			copy(dst[o:], src[j:hi])
		}

		src, dst = dst, src
	}

	copy(entries, src)
}

// compare is fmtsort's compare, reading members only up to the first that
// differs.
func (k *keyOrder) compare(a, b reflect.Value) int {
	k.reads++

	stack := append(k.stack[:0], keyFrame{a: a, b: b, n: -1})

	defer func() { k.stack = stack[:0] }()

	for len(stack) > 0 {
		f := &stack[len(stack)-1]

		if f.n >= 0 {
			if f.i == f.n {
				stack = stack[:len(stack)-1]

				continue
			}

			k.reads++
			x, y := keyMember(f.a, f.i), keyMember(f.b, f.i)
			f.i++
			stack = append(stack, keyFrame{a: x, b: y, n: -1})

			continue
		}

		a, b := f.a, f.b
		stack = stack[:len(stack)-1]

		if a.Type() != b.Type() {
			return -1
		}

		c := 0

		switch a.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			c = cmp.Compare(a.Int(), b.Int())
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			c = cmp.Compare(a.Uint(), b.Uint())
		case reflect.String:
			c = cmp.Compare(a.String(), b.String())
		case reflect.Float32, reflect.Float64:
			c = cmp.Compare(a.Float(), b.Float())
		case reflect.Complex64, reflect.Complex128:
			x, y := a.Complex(), b.Complex()

			if c = cmp.Compare(real(x), real(y)); c == 0 {
				c = cmp.Compare(imag(x), imag(y))
			}
		case reflect.Bool:
			switch x, y := a.Bool(), b.Bool(); {
			case x == y:
			case x:
				c = 1
			default:
				c = -1
			}
		case reflect.Pointer, reflect.UnsafePointer:
			c = cmp.Compare(a.Pointer(), b.Pointer())
		case reflect.Chan:
			if n, ok := nilCompare(a, b); ok {
				c = n
			} else {
				c = cmp.Compare(a.Pointer(), b.Pointer())
			}
		case reflect.Struct:
			stack = append(stack, keyFrame{a: a, b: b, n: a.NumField()})
		case reflect.Array:
			stack = append(stack, keyFrame{a: a, b: b, n: a.Len()})
		case reflect.Interface:
			if n, ok := nilCompare(a, b); ok {
				c = n

				break
			}

			x, y := reflect.ValueOf(a.Elem().Type()), reflect.ValueOf(b.Elem().Type())

			if c = cmp.Compare(x.Pointer(), y.Pointer()); c == 0 {
				k.reads++
				stack = append(stack, keyFrame{a: a.Elem(), b: b.Elem(), n: -1})
			}
		}

		if c != 0 {
			return c
		}
	}

	return 0
}

func nilCompare(a, b reflect.Value) (int, bool) {
	switch {
	case a.IsNil() && b.IsNil():
		return 0, true
	case a.IsNil():
		return -1, true
	case b.IsNil():
		return 1, true
	}

	return 0, false
}
