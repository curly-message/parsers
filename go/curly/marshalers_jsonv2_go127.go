//go:build goexperiment.jsonv2 && go1.27

package curly

import (
	jsonv2 "encoding/json/v2"
	"reflect"
)

// marshalerTo is the interface of MarshalJSONTo. Go 1.27 lets a file name
// encoding/json/v2 only at language version 1.27, which the go1.27
// constraint sets, so Go 1.26 builds marshalers_jsonv2_go126.go instead.
var marshalerTo = reflect.TypeFor[jsonv2.MarshalerTo]()
