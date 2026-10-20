//go:build !goexperiment.jsonv2

package curly

import (
	"encoding"
	"encoding/json"
	"reflect"
)

// marshalers are the methods encoding/json describes a value by, where the
// toolchain builds it on its own: before Go 1.27, unless GOEXPERIMENT=jsonv2.
var marshalers = []reflect.Type{reflect.TypeFor[json.Marshaler](), reflect.TypeFor[encoding.TextMarshaler]()}
