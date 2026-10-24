package main

import (
	"slices"
	"testing"
)

func TestOldFullCopyGetsNewDefaults(t *testing.T) {
	old := `{"topbar":{"items":["title","live","view","sort","reverse","kanbanMode","filters","search","agents","spacer","addGroup","arrange","appearance","settings"]},
	        "card":{"actions":["image","terminal","copy"]}}`
	s, err := ParseSettings("x", []byte(old))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(StringList(s.Raw, "topbar.items"), "remote") || !slices.Contains(StringList(s.Raw, "card.actions"), "info") {
		t.Fatalf("untouched old lists should follow the current defaults: %v %v", StringList(s.Raw, "topbar.items"), StringList(s.Raw, "card.actions"))
	}
}

func TestCustomListsAreKept(t *testing.T) {
	s, err := ParseSettings("x", []byte(`{"topbar":{"items":["title","filters"]},"card":{"actions":["copy"]}}`))
	if err != nil {
		t.Fatal(err)
	}
	if got := StringList(s.Raw, "topbar.items"); !slices.Equal(got, []string{"title", "filters"}) {
		t.Fatalf("custom items replaced: %v", got)
	}
	if got := StringList(s.Raw, "card.actions"); !slices.Equal(got, []string{"copy"}) {
		t.Fatalf("custom actions replaced: %v", got)
	}
}

func TestCurrentDefaultsAreNotRetired(t *testing.T) {
	def := defaults()
	for path, olds := range retiredLists {
		cur := StringList(def, path)
		for _, o := range olds {
			if slices.Equal(cur, o) {
				t.Errorf("%s: the current default is listed as retired", path)
			}
		}
	}
}
