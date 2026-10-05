package als

import (
	"path/filepath"
	"testing"
)

func TestPluginRefs(t *testing.T) {
	s, err := Load(filepath.Join("..", "..", "testdata", "live", "SampleAbletonProject_v2.als"))
	if err != nil {
		t.Skip(err)
	}
	ps := s.PluginRefs()
	if len(ps) != 1 || ps[0].Name != "Vital" || ps[0].Format != "VST3" || ps[0].UID != "56535456-6974-6176-6974-616c00000000" {
		t.Fatalf("%+v", ps)
	}
	if v := s.Version(); v == "" || v[0] < '0' || v[0] > '9' {
		t.Errorf("version %q from %q", v, s.Creator())
	}
}
