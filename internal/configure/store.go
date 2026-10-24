package configure

import "github.com/war-apps/nerv-by-gentle-ai/internal/configstore"

// Store reads and writes the user-scope config.yml on the real filesystem.
// It is an alias for the shared disk-access seam every configuration reader or
// writer (configure, install, wizard) depends on downward, instead of
// install depending upward on configure just to read the same file (see
// internal/configstore).
type Store = configstore.Store
