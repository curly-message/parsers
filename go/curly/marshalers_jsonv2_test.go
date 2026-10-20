//go:build go1.27 && goexperiment.jsonv2

package curly

import (
	"encoding/json/jsontext"
	"testing"
)

type toOf struct{ F func() }

func (toOf) MarshalJSONTo(e *jsontext.Encoder) error { return e.WriteToken(jsontext.String("o")) }

// Built on encoding/json/v2, encoding/json describes a value by its
// MarshalJSONTo method as well, and by AppendText, each at most hostDepth
// levels deep as any other value it is handed.
func TestHostMarshalJSONTo(t *testing.T) {
	if text, want, ok := describedAsEncodingJSON(toOf{func() {}}); !ok || want != `["o"]` {
		t.Errorf("got %q, encoding/json %q", text, want)
	}

	atHostDepth(t, "a MarshalJSONTo leaf", leafAt(toOf{}, 1))
	atHostDepth(t, "an AppendText leaf", leafAt(appendOf{}, 1))
}
