package project

import (
	"testing"
)

func TestInventory(t *testing.T) {
	r, _ := Init(newProject(t), "yi")
	inv, err := r.Inventory()
	if err != nil {
		t.Fatal(err)
	}
	if len(inv.Sets) != 1 || inv.Sets[0].Path != "Song.als" || inv.Sets[0].Version == "" {
		t.Fatalf("sets %+v", inv.Sets)
	}
	if ps := inv.Sets[0].Plugins; len(ps) != 1 || ps[0].Name != "Vital" {
		t.Fatalf("plugins %+v", ps)
	}
	if inv.Files == 0 || inv.Bytes == 0 {
		t.Fatalf("files %d (%d bytes), ignored %d", inv.Files, inv.Bytes, inv.Ignored)
	}
	if inv.Samples.InProject == 0 {
		t.Errorf("samples %+v", inv.Samples)
	}
	if hs, _ := r.Store.List(); len(hs) != 0 {
		t.Error("an inventory stores nothing")
	}
}
