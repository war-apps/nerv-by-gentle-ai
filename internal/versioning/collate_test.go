package versioning

import (
	"slices"
	"testing"
)

// The order node 24 gives these strings with localeCompare (ICU root), the
// comparison conventional-changelog-writer sorts commits with; the
// strings are scope+subject keys from dim's CHANGELOG files and probes of
// punctuation, digits, case, accents and ß.
func TestLocaleCompareMatchesNode(t *testing.T) {
	want := []string{
		"a b", "a_b", "a-b", "a.b", "a0b", "aab", "aAb", "Ab", "ab10", "ab2",
		"AFIP-phase parity WU1", "AFIP-phase parity WU2", "cipin GitHub's", "dockerinstall wget",
		"Gestión-phase parity", "keep worker phases",
		"provider-billingaccept the Matanza", "provider-billingadd AMX offline", "provider-billingAGIP ABL",
		"provider-billingARBA client", "provider-billingARBA Complementario", "receiptscarry",
		"resume", "Resume", "résumé", "strasse", "straße", "test-envaddress fourth", "test-envaddress review",
	}
	got := slices.Clone(want)
	slices.Reverse(got)
	slices.SortStableFunc(got, localeCompare)
	if !slices.Equal(got, want) {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
}
