package migrate

import (
	"bytes"
	"strings"

	"github.com/war-apps/nerv-by-gentle-ai/internal/config"
	"github.com/war-apps/nerv-by-gentle-ai/internal/releaseconfig"
	"github.com/war-apps/nerv-by-gentle-ai/internal/releaseconfig/yamlsection"
	"gopkg.in/yaml.v3"
)

// migrateShip converts the 5.0.0-alpha.1 ship: section of doc, in place,
// into today's release: section: the key is renamed and its keys move
// where the version and build slots put them (releaseconfig.MoveKeys). A
// file that has a release: section too is refused: nerv migrate does not
// merge them (where names the file).
func migrateShip(p *Plan, doc *config.Document, where string) error {
	ship, release := releaseconfig.AlphaSectionKey, releaseconfig.SectionKey
	if doc.KeyExists(release) {
		return refuse("%s has both a ship: section (5.0.0-alpha.1) and a release: section: nerv migrate does not merge them; move what the ship: section still holds into the release: section by hand and remove it, or remove the release: section and run nerv migrate again", where)
	}
	node, err := yamlsection.Locate([]byte(doc.String()), ship)
	if err != nil {
		return refuse("ship: cannot be read: %v", err)
	}
	if node.ShortTag() == "!!null" {
		node = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	}
	if node.Kind != yaml.MappingNode {
		return refuse("ship: must be a mapping")
	}
	notes, err := moveKeys(node)
	if err != nil {
		return err
	}
	block, err := sectionBlock(release, node)
	if err != nil {
		return err
	}
	if doc.KeyIsInline(ship) {
		return refuse("ship: is written on one line: nerv migrate converts a block section only; rename it release: and move its keys by hand (see docs/migration.md)")
	}
	// The block replaces ship: where it was, with the comment of its line.
	if text, _ := doc.Block(ship); text != "" {
		first, _, _ := strings.Cut(text, "\n")
		if i := strings.Index(first, "#"); i >= 0 {
			key, rest, _ := strings.Cut(block, "\n")
			block = strings.TrimRight(key, " ") + " " + first[i:] + "\n" + rest
		}
	}
	doc.SetBlock(ship, strings.TrimRight(block, "\n"))
	p.note("ship: renamed release: (the 5.0.0-alpha.1 section)")
	for _, n := range notes {
		p.note("%s", n)
	}
	return nil
}

// moveReleaseKeys moves the keys of a release: section written between the
// ship -> release rename and the version and build slots (engine keys at
// its top, a project's build: mapping), as Load's refusal of them says.
// A section with nothing to move is left exactly as it is.
func moveReleaseKeys(p *Plan, doc *config.Document) error {
	node, err := yamlsection.Locate([]byte(doc.String()), releaseconfig.SectionKey)
	if err != nil || node.Kind != yaml.MappingNode || doc.KeyIsInline(releaseconfig.SectionKey) {
		// Load reports what is wrong with it.
		return nil
	}
	notes, err := moveKeys(node)
	if err != nil || len(notes) == 0 {
		return err
	}
	block, err := sectionBlock(releaseconfig.SectionKey, node)
	if err != nil {
		return err
	}
	doc.SetBlock(releaseconfig.SectionKey, strings.TrimRight(block, "\n"))
	for _, n := range notes {
		p.note("%s", n)
	}
	return nil
}

// moveKeys is releaseconfig.MoveKeys, its errors refusals.
func moveKeys(node *yaml.Node) ([]string, error) {
	notes, err := releaseconfig.MoveKeys(node)
	if err != nil {
		return nil, refuse("%v", err)
	}
	return notes, nil
}

// sectionBlock is the "<key>:" block text of node.
func sectionBlock(key string, node *yaml.Node) (string, error) {
	var b bytes.Buffer
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	root := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: []*yaml.Node{scalar(key), node}}
	if err := enc.Encode(root); err != nil {
		return "", err
	}
	if err := enc.Close(); err != nil {
		return "", err
	}
	return b.String(), nil
}
