//go:build goexperiment.jsonv2

package curly

import (
	"encoding"
	"encoding/json"
	"reflect"
)

// marshalers are the methods encoding/json describes a value by, where the
// toolchain builds it on encoding/json/v2: from Go 1.27, or with
// GOEXPERIMENT=jsonv2. It calls AppendText and MarshalJSONTo as well.
var marshalers = []reflect.Type{
	reflect.TypeFor[json.Marshaler](),
	reflect.TypeFor[encoding.TextMarshaler](),
	reflect.TypeFor[encoding.TextAppender](),
	marshalerTo,
}
