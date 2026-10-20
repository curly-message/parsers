package curly

import (
	"reflect"
	"strconv"
)

var (
	stringType    = reflect.TypeFor[string]()
	undefinedType = reflect.TypeFor[Undefined]()
)

// plainObject reports whether t is the host's plain object (section 4): an
// unnamed map keyed by the predeclared string. It is a question about the
// value's own type, so a named map type says what it is through its own
// conversion, as does a map keyed by a named string type.
func plainObject(t reflect.Type) bool {
	return t.Kind() == reflect.Map && t.Name() == "" && t.Key() == stringType
}

// plainArray reports whether t is the host's plain array: an unnamed slice
// of anything but bytes, since bytes are binary data, which is something
// more specific than an ordered sequence: a slice encoding/json writes in
// base64.
func plainArray(t reflect.Type) bool {
	return t.Kind() == reflect.Slice && t.Name() == "" && !isBytes(t)
}

// scalar reports whether a value of kind k converts by section 3.3 or 3.4
// directly, where its type has no methods: no host code runs, and the
// answer is the same every time.
func scalar(k reflect.Kind) bool {
	switch k {
	case reflect.Bool, reflect.String, reflect.Float32, reflect.Float64,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return true
	}

	return false
}

func appendScalar(dst []byte, v reflect.Value) []byte {
	switch v.Kind() {
	case reflect.Bool:
		return strconv.AppendBool(dst, v.Bool())
	case reflect.String:
		return append(dst, v.String()...)
	case reflect.Float32:
		return appendFloat(dst, v.Float(), 32)
	case reflect.Float64:
		return appendFloat(dst, v.Float(), 64)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.AppendInt(dst, v.Int(), 10)
	}

	return strconv.AppendUint(dst, v.Uint(), 10)
}

// site is where a value was read: a map and the key it was read under. A
// value with no identity of its own is recorded by where it was read, so
// one entry read by any number of placeholders converts once, while two
// copies of a Go value under two keys are two values.
type site struct {
	m   uintptr
	key string
}

var noSite = site{}

func siteOf(m reflect.Value, key string) site {
	return site{m.Pointer(), key}
}

type recordKey struct {
	t        reflect.Type
	p        uintptr
	len, cap int
	site     site
}

// recorded keeps the value it describes, so that the identity it is
// recorded under stays the value's for as long as the record lives.
type recorded struct {
	value any
	text  string
	ok    bool
}

// recordKeyOf is what a value is recorded under: a map, a pointer or a
// channel by its type and address, a slice by those and its length and
// capacity, anything else by where it was read. Keying by the value itself
// would miss on a value holding NaN, which never equals itself, and a func
// compares by its code, which two closures share.
func recordKeyOf(v reflect.Value, s site) (recordKey, bool) {
	switch v.Kind() {
	case reflect.Map, reflect.Pointer, reflect.Chan:
		return recordKey{t: v.Type(), p: v.Pointer()}, true
	case reflect.Slice:
		return recordKey{t: v.Type(), p: v.Pointer(), len: v.Len(), cap: v.Cap()}, true
	}

	return recordKey{site: s}, s != noSite
}

// convert is the text a value resolves to (section 4), and false where no
// conversion describes it: a plain object or a plain array becomes JSON, a
// scalar without methods its own text, anything else what fmt's %v makes of
// it. A conversion that walks a value or runs host code is recorded for the
// resolution, its failure included, so one value converts once.
func (w *walk) convert(v any, s site) (string, bool) {
	switch x := v.(type) {
	case nil:
		return "<nil>", true
	case Undefined:
		return "", false
	case string:
		return x, true
	case float64:
		return string(appendFloat(nil, x, 64)), true
	case int:
		return strconv.Itoa(x), true
	case bool:
		return strconv.FormatBool(x), true
	}

	rv := reflect.ValueOf(v)
	t := rv.Type()

	if t.NumMethod() == 0 && scalar(t.Kind()) {
		return string(appendScalar(nil, rv)), true
	}

	key, keyed := recordKeyOf(rv, s)

	if keyed {
		if r, ok := w.record[key]; ok {
			return r.text, r.ok
		}
	}

	c := conversion{w: w}

	var ok bool

	if plainObject(t) || plainArray(t) {
		ok = c.json(rv)
	} else {
		ok = c.print(v)
	}

	text := ""

	if ok {
		text = string(c.buf)
	}

	if keyed {
		if w.record == nil {
			w.record = map[recordKey]recorded{}
		}

		w.record[key] = recorded{v, text, ok}
	}

	return text, ok
}

// conversion is one conversion of one value, and what it may spend: every
// value it visits is a node, a member and an element each one, and a
// conversion visiting more than MaxConversionNodes describes nothing.
type conversion struct {
	w      *walk
	nodes  int
	buf    []byte
	fields map[reflect.Type][]jsonField
	keys   keyOrder
	// credit is what is left, to the key being printed, of the nodes its
	// map's sort was charged, which its own visits take up first.
	credit int
}

func (c *conversion) visit() bool {
	if c.credit > 0 {
		c.credit--

		return true
	}

	c.nodes++

	return c.nodes <= MaxConversionNodes
}

// pathID is a container on the path a conversion walks: one met again
// below itself is a cycle, which no conversion describes.
type pathID struct {
	t reflect.Type
	p uintptr
	n int
}

func pathOf(v reflect.Value) pathID {
	id := pathID{t: v.Type(), p: v.Pointer()}

	if v.Kind() == reflect.Slice {
		id.n = v.Len()
	}

	return id
}

// unwrap is the value an interface holds, and the invalid Value for nil.
func unwrap(v reflect.Value) reflect.Value {
	if v.Kind() == reflect.Interface {
		return v.Elem()
	}

	return v
}

func isUndefined(v reflect.Value) bool {
	return v.IsValid() && v.Type() == undefinedType
}
