package compose

import "testing"

func TestPathKey(t *testing.T) {
	a, b := `C:\Work\App\.env`, `c:\work\app\.ENV`
	if pathKey(a, true) != pathKey(b, true) {
		t.Errorf("folded keys differ: %q vs %q", pathKey(a, true), pathKey(b, true))
	}
	if pathKey(a, false) == pathKey(b, false) {
		t.Error("unfolded keys must stay case-sensitive")
	}
}
