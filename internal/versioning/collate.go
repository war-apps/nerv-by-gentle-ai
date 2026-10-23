package versioning

import (
	"sync"

	"golang.org/x/text/collate"
	"golang.org/x/text/language"
)

// rootCollator is the CLDR root collation that JavaScript's
// String.prototype.localeCompare uses under node's default ICU locale (en-US
// and C.UTF-8 share it), which conventional-changelog-writer sorts commits
// with: three levels, punctuation and symbols non-ignorable and ordered
// before digits, digits before letters. A collate.Collator is not safe for
// concurrent use, hence the mutex.
var (
	rootCollatorMu sync.Mutex
	rootCollator   = collate.New(language.Und)
)

// localeCompare compares a and b as node's localeCompare does.
func localeCompare(a, b string) int {
	rootCollatorMu.Lock()
	defer rootCollatorMu.Unlock()
	return rootCollator.CompareString(a, b)
}
