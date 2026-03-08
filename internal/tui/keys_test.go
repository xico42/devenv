package tui

import "testing"

func TestKeyMap_ShortHelp(t *testing.T) {
	km := defaultKeyMap()
	bindings := km.ShortHelp()
	if len(bindings) == 0 {
		t.Fatal("ShortHelp returned no bindings")
	}
	// Verify key actions are present.
	keys := make(map[string]bool)
	for _, b := range bindings {
		keys[b.Help().Key] = true
	}
	for _, want := range []string{"a/enter", "s", "n", "d", "q"} {
		if !keys[want] {
			t.Errorf("ShortHelp missing key %q", want)
		}
	}
}
