package shipconfig

import (
	"errors"

	"github.com/war-apps/nerv-by-gentle-ai/internal/shipconfig/yamlsection"
	"gopkg.in/yaml.v3"
)

// locateRelease returns the value node of the top-level release: key (see
// yamlsection.Locate for how the block is cut out of the file). It returns
// ErrNotConfigured when there is none, a *ValidationError for a document
// whose top level is not a mapping or an alias to an anchor outside the
// section, and a plain error for a YAML syntax error inside it.
func locateRelease(data []byte) (*yaml.Node, error) {
	node, err := yamlsection.Locate(data, "release")
	var problem *yamlsection.ProblemError
	switch {
	case errors.Is(err, yamlsection.ErrAbsent):
		return nil, ErrNotConfigured
	case errors.As(err, &problem):
		return nil, &ValidationError{Problems: []string{problem.Problem}}
	}
	return node, err
}
