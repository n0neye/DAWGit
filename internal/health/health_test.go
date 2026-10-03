package health

import (
	"testing"
	"time"

	"dawgit/internal/als"
	"dawgit/internal/liveenv"
	"dawgit/internal/project"
)

func TestCheck(t *testing.T) {
	inv := &project.Inventory{Files: 10, Bytes: 1000,
		Sets: []project.SetInventory{
			{Path: "Song.als", Version: "12.1.5", Plugins: []als.PluginRef{
				{Name: "Vital", Format: "VST3", UID: "56535456-6974-6176-6974-616c00000000"},
				{Name: "Serum_x64", Format: "VST", UID: "1483109208", File: "Serum_x64.dll"}}},
			{Path: "Alt.als", Version: "12.3.1", Plugins: []als.PluginRef{
				{Name: "Vital", Format: "VST3", UID: "56535456-6974-6176-6974-616c00000000"},
				{Name: "Kontakt 8", Format: "VST3", UID: "aaaa"}}},
		},
		Samples: project.SampleInventory{InProject: 5, External: []project.FileEntry{{Path: "D:/x.wav", Size: 7}},
			Packs: []string{"Core Library", "Drum Booth"}, PackRefs: 3, Missing: []string{"Samples/gone.wav"}}}
	env := &liveenv.Env{
		Installs:     []liveenv.Install{{Name: "Ableton Live 12 Suite", Version: "12.3.2", Edition: "Suite"}},
		PluginsKnown: true, PacksKnown: true, Packs: []string{"Core Library"},
		Plugins: []liveenv.Plugin{
			{ID: "device:vst3:instr:56535456-6974-6176-6974-616C00000000", Format: "VST3", Name: "Vital", Vendor: "Vital Audio", Version: "1.0.7"},
			{ID: "device:vst:instr:1483109208", Format: "VST", Name: "Serum", File: `C:\VST\Serum_x64.dll`, Version: "1.3"},
		}}
	r := Check(inv, env)
	l := r.Live
	if l == nil || l.Needs != "12.3.1" || l.Opens != "yes" {
		t.Fatalf("live %+v", l)
	}
	if len(l.Plugins) != 3 {
		t.Fatalf("plugins %+v", l.Plugins)
	}
	got := map[string]PluginLine{}
	for _, p := range l.Plugins {
		got[p.Name] = p
	}
	if v := got["Vital"]; v.Here != "yes" || v.Version != "1.0.7" || len(v.Sets) != 2 {
		t.Errorf("vital %+v", v)
	}
	if got["Serum_x64"].Here != "yes" || got["Kontakt 8"].Here != "no" {
		t.Errorf("serum %+v kontakt %+v", got["Serum_x64"], got["Kontakt 8"])
	}
	if s := l.Samples; s.External != 1 || s.ExternalBytes != 7 || len(s.Packs) != 2 || s.Packs[0].Here != "yes" ||
		s.Packs[1].Here != "no" || len(s.Missing) != 1 {
		t.Errorf("samples %+v", s)
	}

	// An older Live here; nothing known about plugins.
	env.Installs[0].Version = "12.0.5"
	env.PluginsKnown = false
	l = Check(inv, env).Live
	if l.Opens != "older" || l.Plugins[0].Here != "unknown" {
		t.Errorf("older: %s, %+v", l.Opens, l.Plugins[0])
	}
	env.Installs = nil
	if Check(inv, env).Live.Opens != "none" {
		t.Error("no Live")
	}
	if Check(&project.Inventory{Files: 3}, env).Live != nil {
		t.Error("no sets: no Live report")
	}
}

func TestCheckTeam(t *testing.T) {
	vital := als.PluginRef{Name: "Vital", Format: "VST3", UID: "56535456-6974-6176-6974-616c00000000"}
	serum := als.PluginRef{Name: "Serum 2", Format: "VST3", UID: "bbbb"}
	inv := &project.Inventory{Sets: []project.SetInventory{{Path: "Song.als", Version: "12.3.1",
		Plugins: []als.PluginRef{vital, serum}}}, Samples: project.SampleInventory{Packs: []string{"Drum Booth"}, PackRefs: 2}}
	here := &liveenv.Env{Installs: []liveenv.Install{{Version: "12.3.2"}}, PluginsKnown: true, PacksKnown: true,
		Packs: []string{"Drum Booth"}, Plugins: []liveenv.Plugin{
			{ID: "device:vst3:instr:" + vital.UID, Format: "VST3", Name: "Vital", Version: "1.0.7"},
			{ID: "device:vst3:instr:bbbb", Format: "VST3", Name: "Serum 2", Version: "2.0.1"}}}
	mika := SetupFrom(&liveenv.Env{Installs: []liveenv.Install{{Version: "12.1.5", Edition: "Standard"}},
		PluginsKnown: true, PacksKnown: true, Plugins: []liveenv.Plugin{
			{ID: "device:vst3:instr:" + vital.UID, Format: "VST3", Name: "Vital", Version: "1.0.6"}}}, time.Now())
	rep := Check(inv, here)
	CheckTeam(rep, inv, map[string]Setup{"Mika": mika})
	if len(rep.Team) != 1 {
		t.Fatal(rep.Team)
	}
	m := rep.Team[0]
	if m.Live != "12.1.5" || m.Opens != "older" || len(m.MissingPlugins) != 1 || m.MissingPlugins[0] != "Serum 2" ||
		len(m.OtherVersions) != 1 || m.OtherVersions[0] != (PluginVersion{"Vital", "1.0.6", "1.0.7"}) ||
		len(m.MissingPacks) != 1 {
		t.Fatalf("%+v", m)
	}
	again := SetupFrom(here, time.Now().Add(time.Hour))
	if !again.Same(SetupFrom(here, time.Now())) || again.Same(mika) {
		t.Error("Same")
	}
}
