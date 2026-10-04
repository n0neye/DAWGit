package unity

import (
	"strings"
	"testing"
)

// prefab is a small prefab as Unity 6 saves it: a root with one child.
// The placeholders are filled by edit.
const prefab = `%YAML 1.1
%TAG !u! tag:unity3d.com,2011:
--- !u!1 &1000000000000000001
GameObject:
  m_ObjectHideFlags: 0
  m_CorrespondingSourceObject: {fileID: 0}
  m_PrefabInstance: {fileID: 0}
  m_PrefabAsset: {fileID: 0}
  serializedVersion: 6
  m_Component:
  - component: {fileID: 1000000000000000002}
  m_Layer: 0
  m_Name: {{rootName}}
  m_TagString: Untagged
  m_Icon: {fileID: 0}
  m_NavMeshLayer: 0
  m_StaticEditorFlags: 0
  m_IsActive: 1
--- !u!4 &1000000000000000002
Transform:
  m_ObjectHideFlags: 0
  m_CorrespondingSourceObject: {fileID: 0}
  m_PrefabInstance: {fileID: 0}
  m_PrefabAsset: {fileID: 0}
  m_GameObject: {fileID: 1000000000000000001}
  serializedVersion: 2
  m_LocalRotation: {x: 0, y: 0, z: 0, w: 1}
  m_LocalPosition: {x: {{rootX}}, y: 0, z: 0}
  m_LocalScale: {x: 1, y: 1, z: 1}
  m_ConstrainProportionsScale: 0
  m_Children:
  - {fileID: 1000000000000000004}
  m_Father: {fileID: 0}
  m_LocalEulerAnglesHint: {x: 0, y: 0, z: 0}
--- !u!1 &1000000000000000003
GameObject:
  m_ObjectHideFlags: 0
  m_CorrespondingSourceObject: {fileID: 0}
  m_PrefabInstance: {fileID: 0}
  m_PrefabAsset: {fileID: 0}
  serializedVersion: 6
  m_Component:
  - component: {fileID: 1000000000000000004}
  m_Layer: 0
  m_Name: {{childName}}
  m_TagString: Untagged
  m_Icon: {fileID: 0}
  m_NavMeshLayer: 0
  m_StaticEditorFlags: 0
  m_IsActive: 1
--- !u!4 &1000000000000000004
Transform:
  m_ObjectHideFlags: 0
  m_CorrespondingSourceObject: {fileID: 0}
  m_PrefabInstance: {fileID: 0}
  m_PrefabAsset: {fileID: 0}
  m_GameObject: {fileID: 1000000000000000003}
  serializedVersion: 2
  m_LocalRotation: {x: 0, y: 0, z: 0, w: 1}
  m_LocalPosition: {x: 0, y: 0, z: 0}
  m_LocalScale: {x: 1, y: 1, z: 1}
  m_ConstrainProportionsScale: 0
  m_Children: []
  m_Father: {fileID: 1000000000000000002}
  m_LocalEulerAnglesHint: {x: 0, y: 0, z: 0}
`

func edit(rootName, rootX, childName string) []byte {
	return []byte(strings.NewReplacer("{{rootName}}", rootName, "{{rootX}}", rootX, "{{childName}}", childName).Replace(prefab))
}

func needYAMLMerge(t *testing.T) {
	if findYAMLMerge() == "" {
		t.Skip("Unity isn't installed")
	}
}

func TestMergeYAMLCombines(t *testing.T) {
	needYAMLMerge(t)
	base := edit("Ship", "0", "Engine")
	ours := edit("Ship", "5", "Engine")     // moved the root
	theirs := edit("Ship", "0", "Thruster") // renamed the child
	got, clean, err := mergeYAML(base, ours, theirs)
	if err != nil || !clean {
		t.Fatalf("clean = %v, err = %v", clean, err)
	}
	s := string(got)
	if !strings.Contains(s, "m_LocalPosition: {x: 5, y: 0, z: 0}") || !strings.Contains(s, "m_Name: Thruster") ||
		!strings.Contains(s, "m_Name: Ship") {
		t.Fatalf("both edits should be kept:\n%s", s)
	}
}

func TestMergeYAMLConflict(t *testing.T) {
	needYAMLMerge(t)
	base := edit("Ship", "0", "Engine")
	_, clean, err := mergeYAML(base, edit("Ship", "5", "Engine"), edit("Ship", "7", "Engine"))
	if err != nil || clean {
		t.Fatalf("the same property changed both ways: clean = %v, err = %v", clean, err)
	}
}

func TestMergeYAMLNotText(t *testing.T) {
	// Binary-serialized assets, or no Unity: never clean, never an error.
	_, clean, err := mergeYAML([]byte("\x00\x01"), []byte("\x00\x02"), []byte("\x00\x03"))
	if err != nil || clean {
		t.Fatalf("clean = %v, err = %v", clean, err)
	}
	old := findYAMLMerge
	defer func() { findYAMLMerge = old }()
	findYAMLMerge = func() string { return "" }
	base := edit("Ship", "0", "Engine")
	if _, clean, err := mergeYAML(base, edit("Ship", "5", "Engine"), edit("Ship", "0", "Thruster")); err != nil || clean {
		t.Fatalf("without Unity: clean = %v, err = %v", clean, err)
	}
}

func TestVersionLess(t *testing.T) {
	for _, c := range [][2]string{{"2021.3.45f1", "2022.3.15f1"}, {"2022.3.15f1", "6000.0.40f1"},
		{"6000.0.40f1", "6000.0.59f2"}, {"6000.0.59f2", "6000.3.12f1"}, {"6000.3.9f1", "6000.3.12f1"}} {
		if !versionLess(c[0], c[1]) || versionLess(c[1], c[0]) {
			t.Errorf("%s < %s", c[0], c[1])
		}
	}
}
