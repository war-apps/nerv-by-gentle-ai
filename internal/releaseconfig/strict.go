package releaseconfig

import (
	"reflect"

	"github.com/war-apps/nerv-by-gentle-ai/internal/releaseconfig/yamlsection"
	"gopkg.in/yaml.v3"
)

var typeOfRelease = reflect.TypeFor[Release]()

// unknownFields reports every key of the release: node that Release does
// not declare (see yamlsection.UnknownFields).
func unknownFields(node *yaml.Node) ([]string, error) {
	return yamlsection.UnknownFields(node, typeOfRelease, SectionKey)
}
