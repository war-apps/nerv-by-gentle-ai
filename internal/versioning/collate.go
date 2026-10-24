package versioning

import "strings"

// localeCompare approximates JavaScript's String.prototype.localeCompare
// under node's default ICU root collation, which conventional-changelog-
// writer sorts commits with: three levels (base character, accent, case,
// lowercase first), with spaces and punctuation non-ignorable and ordered
// before digits, and digits before letters. It covers ASCII and the Latin-1
// and Latin Extended-A letters; other characters sort after letters by code
// point.
func localeCompare(a, b string) int {
	ka, kb := collationKey(a), collationKey(b)
	for level := 0; level < 3; level++ {
		if c := compareWeights(ka[level], kb[level]); c != 0 {
			return c
		}
	}
	return 0
}

func compareWeights(a, b []int) int {
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] != b[i] {
			if a[i] < b[i] {
				return -1
			}
			return 1
		}
	}
	switch {
	case len(a) < len(b):
		return -1
	case len(a) > len(b):
		return 1
	}
	return 0
}

// rootPunctuation is the CLDR root order of ASCII whitespace, punctuation
// and symbols, all before the digits.
const rootPunctuation = "\t\n\v\f\r _-,;:!?.'\"()[]{}@*/\\&#%`^+<=>|~$"

// latinFolds maps accented Latin letters to their base letter and an accent
// weight (secondary level).
var latinFolds = func() map[rune][2]int {
	m := map[rune][2]int{}
	groups := []struct {
		base    rune
		letters string
	}{
		{'a', "áàâäãåāăą"}, {'c', "çćĉċč"}, {'d', "ďđ"}, {'e', "éèêëēĕėęě"},
		{'g', "ĝğġģ"}, {'h', "ĥħ"}, {'i', "íìîïĩīĭįı"}, {'j', "ĵ"}, {'k', "ķ"},
		{'l', "ĺļľŀł"}, {'n', "ñńņňŉ"}, {'o', "óòôöõøōŏő"}, {'r', "ŕŗř"},
		{'s', "śŝşš"}, {'t', "ţťŧ"}, {'u', "úùûüũūŭůűų"}, {'w', "ŵ"},
		{'y', "ýÿŷ"}, {'z', "źżž"},
	}
	for _, g := range groups {
		for i, r := range []rune(g.letters) {
			m[r] = [2]int{int(g.base), i + 1}
			if up := []rune(strings.ToUpper(string(r))); len(up) == 1 && up[0] != r {
				m[up[0]] = [2]int{int(g.base), i + 1}
			}
		}
	}
	return m
}()

// collationKey returns the primary, secondary and tertiary weights of s.
func collationKey(s string) [3][]int {
	var k [3][]int
	add := func(primary, secondary, tertiary int) {
		k[0] = append(k[0], primary)
		k[1] = append(k[1], secondary)
		k[2] = append(k[2], tertiary)
	}
	const (
		digitBase  = 100
		letterBase = 200
		otherBase  = 1000
	)
	for _, r := range s {
		switch {
		case strings.ContainsRune(rootPunctuation, r):
			add(1+strings.IndexRune(rootPunctuation, r), 0, 0)
		case r < 0x20 || r == 0x7f:
			// control characters are ignorable
		case r >= '0' && r <= '9':
			add(digitBase+int(r-'0'), 0, 0)
		case r >= 'a' && r <= 'z':
			add(letterBase+int(r-'a'), 0, 0)
		case r >= 'A' && r <= 'Z':
			add(letterBase+int(r-'A'), 0, 1)
		case r == 'ß':
			// ß sorts as ss, after it at the third level.
			add(letterBase+int('s'-'a'), 0, 1)
			add(letterBase+int('s'-'a'), 0, 1)
		default:
			if f, ok := latinFolds[r]; ok {
				upper := 0
				if strings.ToLower(string(r)) != string(r) {
					upper = 1
				}
				add(letterBase+f[0]-'a', f[1], upper)
				continue
			}
			add(otherBase+int(r), 0, 0)
		}
	}
	return k
}
