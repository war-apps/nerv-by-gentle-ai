// Package extensiontest builds the smallest valid extension.yaml and
// provider.yaml manifests, for tests that need an extensions tree without
// depending on what plugins/ ships.
package extensiontest

import (
	"fmt"
	"strings"
)

// Extension is an agent extension.yaml named name, suggested at order, with
// one slot "provider" (cardinality many) whose only operation, "ping", is
// optional.
func Extension(name string, order int, requires ...string) []byte {
	req := ""
	if len(requires) > 0 {
		req = "requires: [" + strings.Join(requires, ", ") + "]\n"
	}
	return []byte(fmt.Sprintf(`name: %s
contract_version: 1
suggest_order: %d
%sruntime: agent
agent: %s:entry
slots:
  - name: provider
    cardinality: many
    operations:
      ping: optional
`, name, order, req, name))
}

// Provider is an agent provider.yaml of the "provider" slot of extension
// ext, named name, that does not implement ping. It lives in the folder
// <ext>-<name>.
func Provider(ext, name string) []byte {
	return []byte(fmt.Sprintf(`name: %s
extension: %s
slot: provider
contract_version: 1
runtime: agent
mcp:
  server: %s
  tools: [%s_ping]
operations:
  ping: not_implemented
`, name, ext, name, name))
}
