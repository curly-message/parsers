package curly

import (
	"cmp"
	"fmt"
	"reflect"
	"slices"
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
	credit int
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

		depth, credit := f.depth+1, 0

		switch f.kind {
		case reflect.Map:
			// A key that printed has taken what it visited of the sort's
			// charge; the rest is left to the keys after it.
			if f.i%2 == 1 {
				f.credit, c.credit = c.credit, 0
			}

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
				child, credit = f.sorted[f.i/2].key, f.credit
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

		if credit > 0 {
			c.credit = credit
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
			c.buf = append(c.buf, v.String()...)
		case reflect.Map:
			id := pathOf(v)

			if path[id] || c.nodes+2*v.Len() > MaxConversionNodes {
				return false
			}

			sorted, credit, ok := c.sortedKeys(v)

			if !ok {
				return false
			}

			c.buf = append(c.buf, "map["...)
			path[id] = true
			*stack = append(*stack, printFrame{v: v, kind: reflect.Map, depth: depth, sorted: sorted, credit: credit, id: id, onPath: true})
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

			if c.nodes-c.credit+v.Len() > MaxConversionNodes {
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
// its key, since a key holding NaN finds nothing when it is looked up.
// Sorting reads every node of every key, a visit the conversion is charged
// for whether or not a method then prints the key, since a map reached
// through many references is sorted at each. It returns the nodes charged,
// which printing the keys then visits uncharged as far as it reaches them,
// so a map's keys cost the larger of the two walks. A map whose keys hold
// more nodes than the conversion has left to visit is refused before
// anything is compared.
func (c *conversion) sortedKeys(m reflect.Value) ([]entry, int, bool) {
	entries := make([]entry, 0, m.Len())
	left := MaxConversionNodes - c.nodes - 2*m.Len()
	charged := 0

	for it := m.MapRange(); it.Next(); {
		key := it.Key()
		n := c.keys.inner(key, left)

		if left -= n; left < 0 {
			return nil, 0, false
		}

		charged += n
		entries = append(entries, entry{key, it.Value()})
	}

	c.nodes += charged

	slices.SortStableFunc(entries, func(a, b entry) int { return c.keys.compare(a.key, b.key) })

	return entries, charged, true
}

// keyOrder walks map keys on one stack for every key of a map, since a key
// can nest structs, arrays and interfaces to any depth.
type keyOrder struct {
	stack []keyFrame
	steps int
}

// keyFrame is a pair of keys to compare, or, where n is set, a pair of
// structs or arrays whose members from i on are still to be compared.
type keyFrame struct {
	a, b reflect.Value
	i, n int
}

// inner counts the nodes below a key, at most one past limit: each field,
// element and interface's value one. A count past the limit stops there
// rather than adding, since a key of zero-size arrays can hold more
// elements than an int counts.
func (k *keyOrder) inner(key reflect.Value, limit int) int {
	count := 0
	stack := append(k.stack[:0], keyFrame{a: key})

	for len(stack) > 0 && count <= limit {
		k.steps++
		v := stack[len(stack)-1].a
		stack = stack[:len(stack)-1]

		switch v.Kind() {
		case reflect.Struct:
			if v.NumField() > limit-count {
				count = limit + 1

				break
			}

			count += v.NumField()

			for i := range v.NumField() {
				stack = append(stack, keyFrame{a: v.Field(i)})
			}
		case reflect.Array:
			if v.Len() > limit-count {
				count = limit + 1

				break
			}

			count += v.Len()

			for i := range v.Len() {
				stack = append(stack, keyFrame{a: v.Index(i)})
			}
		case reflect.Interface:
			if !v.IsNil() {
				stack = append(stack, keyFrame{a: v.Elem()})
			}
		}
	}

	k.stack = stack

	return count
}

// compare is fmtsort's compare, reading members only up to the first that
// differs.
func (k *keyOrder) compare(a, b reflect.Value) int {
	stack := append(k.stack[:0], keyFrame{a: a, b: b, n: -1})

	defer func() { k.stack = stack[:0] }()

	for len(stack) > 0 {
		k.steps++
		f := &stack[len(stack)-1]

		if f.n >= 0 {
			if f.i == f.n {
				stack = stack[:len(stack)-1]

				continue
			}

			var x, y reflect.Value

			if f.a.Kind() == reflect.Struct {
				x, y = f.a.Field(f.i), f.b.Field(f.i)
			} else {
				x, y = f.a.Index(f.i), f.b.Index(f.i)
			}

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
