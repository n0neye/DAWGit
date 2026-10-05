package jsonx

import (
	"encoding/json"
	"strings"
	"testing"
)

type rec struct {
	Name  string   `json:"name"`
	Tags  []string `json:"tags,omitempty"`
	Extra Extra    `json:"-"`
}

func (r *rec) UnmarshalJSON(b []byte) error {
	type plain rec
	return Decode(b, (*plain)(r), &r.Extra)
}

func (r rec) MarshalJSON() ([]byte, error) {
	type plain rec
	return Encode(plain(r), r.Extra)
}

func TestRoundTrip(t *testing.T) {
	in := `{"name":"Band","features":["locks"],"future":{"a":1}}`
	var r rec
	if err := json.Unmarshal([]byte(in), &r); err != nil {
		t.Fatal(err)
	}
	if r.Name != "Band" || len(r.Extra) != 2 {
		t.Fatalf("decoded %+v", r)
	}
	r.Name = "Band 2"
	out, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != `{"name":"Band 2","features":["locks"],"future":{"a":1}}` {
		t.Errorf("encoded %s", out)
	}
	// Inside a slice of a bigger struct, and indented.
	var wrap struct{ Recs []rec }
	json.Unmarshal([]byte(`{"Recs":[`+in+`]}`), &wrap)
	out, _ = json.MarshalIndent(wrap, "", "  ")
	if !strings.Contains(string(out), `"future"`) {
		t.Errorf("nested: %s", out)
	}
	// Known fields win; no extra stays as plain JSON.
	r = rec{Name: "x", Extra: Extra{"name": json.RawMessage(`"old"`)}}
	if out, _ := json.Marshal(r); string(out) != `{"name":"x"}` {
		t.Errorf("known wins: %s", out)
	}
	if out, _ := json.Marshal(rec{Name: "y"}); string(out) != `{"name":"y"}` {
		t.Errorf("plain: %s", out)
	}
}
