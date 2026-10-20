package curly

import "reflect"

// composed is the properties the layers hold under a modifier's name
// (section 11.2): the parser's defaults, then the caller's props, then the
// props of the wrapper the value came in, each overriding only the
// properties it names. The map is a fresh one, so a modifier that writes to
// it reaches neither the next placeholder nor the caller.
func (w *walk) composed(name string, wrapper any) map[string]any {
	out := make(map[string]any, len(w.p.defaults[name]))

	for k, v := range w.p.defaults[name] {
		out[k] = v
	}

	if entry, ok := w.props[name]; ok {
		mergeLayer(out, entry)
	}

	if wrapper != nil {
		if props, _, ok := member(wrapper, "props"); ok {
			if entry, ok := layerEntry(props, name); ok {
				mergeLayer(out, entry)
			}
		}
	}

	return out
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
