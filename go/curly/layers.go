package curly

import "reflect"

// composition is the properties the layers hold under a modifier's name
// (section 11.2), composed property by property: the parser's defaults,
// then the caller's props, then the props of the wrapper the value came in,
// each overriding only the properties it names. Nothing writes to one once
// it is composed.
type composition struct {
	key   composed
	props map[string]any
}

// composed names what a wrapper adds to a modifier's composition: its layer
// under the modifier's name, by the map it is. The zero value is a wrapper
// adding nothing, or no wrapper.
type composed struct {
	layer uintptr
}

// composing is the compositions a resolution keeps, by the modifier they
// are for, the first two in place.
type composing struct {
	first [2]kept
	used  int
	more  map[string]*kept
}

// kept is the compositions a resolution keeps for one modifier. shared is
// what the parser's defaults and the caller's props compose alone, which is
// the same for every placeholder, so a resolution holds one however many
// placeholders ask the modifier. What a wrapper's layer is composed into is
// a copy of the layers beneath it, so of those only last is kept, the one
// composed last: a payload of many wrappers over one wide layer has the
// resolution hold one copy of that layer at a time, never one for each
// wrapper.
type kept struct {
	name   string
	shared composition
	last   *composition
}

// nothing is what composes where no layer holds anything under a name: no
// properties, and no map to hold them.
var nothing composition

// composition is the properties composed under name for a value that came
// in wrapper. It is composed once in a resolution for the layers every
// placeholder shares, and once more for each run of a
// modifier's placeholders whose wrappers hold one layer, and every
// placeholder it serves is handed the same one: what composing costs grows
// with the layers and with the wrappers that name properties, never with
// the placeholders times a layer's width. Nothing of it outlives the walk.
func (w *walk) composition(name string, wrapper any) *composition {
	own := w.entries(name, wrapper)
	key := composed{layer: layerID(own[2])}

	if own[0] == nil && own[1] == nil && own[2] == nil {
		return &nothing
	}

	k := w.kept(name)
	c := &k.shared

	if key != (composed{}) {
		if k.last != nil && k.last.key == key {
			return k.last
		}

		c = &composition{}
		k.last = c
	} else if c.props != nil {
		return c
	}

	c.key = key
	c.props = make(map[string]any, len(w.p.defaults[name]))

	for _, entry := range own {
		mergeLayer(c.props, entry)
	}

	return c
}

// kept is what the resolution keeps for the modifier under name.
func (w *walk) kept(name string) *kept {
	all := w.composing

	if all == nil {
		all = &composing{}
		w.composing = all
	}

	for i := range all.used {
		if all.first[i].name == name {
			return &all.first[i]
		}
	}

	if k := all.more[name]; k != nil {
		return k
	}

	if all.used < len(all.first) {
		all.used++
		all.first[all.used-1].name = name

		return &all.first[all.used-1]
	}

	if all.more == nil {
		all.more = map[string]*kept{}
	}

	k := &kept{name: name}
	all.more[name] = k

	return k
}

// layerID tells one layer from another by the map it is: two wrappers
// holding the same map hold the same layer. What is no layer, and so
// composes nothing, is told from nothing.
func layerID(entry any) uintptr {
	rv, ok := layerOf(entry)

	if !ok {
		return 0
	}

	return rv.Pointer()
}

// entries is what each layer holds under name, the lowest first; a layer
// holding nothing there holds nil.
func (w *walk) entries(name string, wrapper any) [3]any {
	var layers [3]any

	if defaults, ok := w.p.defaults[name]; ok {
		layers[0] = defaults
	}

	layers[1], _ = layerEntry(w.props, name)

	if wrapper != nil {
		if props, _, ok := member(wrapper, "props"); ok {
			layers[2], _ = layerEntry(props, name)
		}
	}

	return layers
}

// layerOf is v as a layer: anything carrying entries by name, which in Go
// is a map keyed by a string type, named or not.
func layerOf(v any) (reflect.Value, bool) {
	if v == nil {
		return reflect.Value{}, false
	}

	rv := reflect.ValueOf(v)

	return rv, rv.Kind() == reflect.Map && rv.Type().Key().Kind() == reflect.String
}

// layerEntry is the entry a layer holds under name.
func layerEntry(layer any, name string) (any, bool) {
	if m, ok := layer.(map[string]any); ok {
		v, present := m[name]

		return v, present
	}

	rv, ok := layerOf(layer)

	if !ok {
		return nil, false
	}

	v := rv.MapIndex(reflect.ValueOf(name).Convert(rv.Type().Key()))

	if !v.IsValid() {
		return nil, false
	}

	return v.Interface(), true
}

// mergeLayer writes the properties a layer names over out, and reports
// whether it was a layer. A property holding Undefined names nothing; one
// holding nil names null.
func mergeLayer(out map[string]any, layer any) bool {
	if m, ok := layer.(map[string]any); ok {
		for k, v := range m {
			if !isUndefinedValue(v) {
				out[k] = v
			}
		}

		return true
	}

	rv, ok := layerOf(layer)

	if !ok {
		return false
	}

	for it := rv.MapRange(); it.Next(); {
		if v := it.Value().Interface(); !isUndefinedValue(v) {
			out[it.Key().String()] = v
		}
	}

	return true
}
