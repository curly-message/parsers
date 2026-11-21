//go:build goexperiment.jsonv2 && !go1.27

package curly

import (
	jsonv2 "encoding/json/v2"
	"reflect"
)

// marshalerTo is the interface of MarshalJSONTo, as
// marshalers_jsonv2_go127.go names it from Go 1.27.
var marshalerTo = reflect.TypeFor[jsonv2.MarshalerTo]()
