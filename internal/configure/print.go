package configure

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"

	"github.com/war-apps/nerv-by-gentle-ai/internal/config"
	"github.com/war-apps/nerv-by-gentle-ai/internal/gentleai"
	"github.com/war-apps/nerv-by-gentle-ai/internal/models"
	"github.com/war-apps/nerv-by-gentle-ai/internal/skills"
)

// OrderedEntry is one key/value pair of PrintResult.Values or
// PrintResult.Defaults.
type OrderedEntry struct {
	Key   string
	Value string
}

// OrderedValues is a key/value list that marshals as a JSON object in list
// order, so PrintResult.Values/Defaults render in config.Defaults'
// catalogue order instead of Go's default alphabetical map ordering.
type OrderedValues []OrderedEntry

// MarshalJSON implements json.Marshaler.
func (v OrderedValues) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, e := range v {
		if i > 0 {
			buf.WriteByte(',')
		}
		key, err := json.Marshal(e.Key)
		if err != nil {
			return nil, err
		}
		val, err := json.Marshal(e.Value)
		if err != nil {
			return nil, err
		}
		buf.Write(key)
		buf.WriteByte(':')
		buf.Write(val)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// ToolFound is a PATH-presence-only prerequisite status (engram, claude).
type ToolFound struct {
	Found bool `json:"found"`
}

// ToolVersion is a version-checked prerequisite status (gentle-ai).
type ToolVersion struct {
	Found bool `json:"found"`
	// Version is nil (JSON null) when Found is false or its output had
	// no parseable MAJOR.MINOR.PATCH token.
	Version *string `json:"version"`
	OK      bool    `json:"ok"`
}

// Prerequisites is -Print's `prerequisites` shape.
type Prerequisites struct {
	GentleAI ToolVersion `json:"gentle_ai"`
	Engram   ToolFound   `json:"engram"`
	Claude   ToolFound   `json:"claude"`
}

// ModelRow is one row of -Print's `models` table.
type ModelRow struct {
	Role   string `json:"role"`
	Model  string `json:"model"`
	Effort string `json:"effort"`
	Source string `json:"source"`
	// Purpose, GentleAIEquivalent and FromPhase come from config.Roles().
	// The equivalent is informational ("" for a role with no gentle-ai
	// counterpart); FromPhase is the only from:<phase> suggestion ("" for
	// none).
	Purpose            string `json:"purpose"`
	GentleAIEquivalent string `json:"gentle_ai_equivalent"`
	FromPhase          string `json:"from_phase"`
}

// SkillStatus is one entry of -Print's `skills_status` array.
type SkillStatus struct {
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Repo      string `json:"repo"`
	Skill     string `json:"skill"`
	Installed bool   `json:"installed"`
	Action    string `json:"action"`
}

// PrintResult is --print's JSON payload: `{ config_path, exists,
// prerequisites, values, defaults, models, skills_status }`.
type PrintResult struct {
	ConfigPath    string        `json:"config_path"`
	Exists        bool          `json:"exists"`
	Prerequisites Prerequisites `json:"prerequisites"`
	Values        OrderedValues `json:"values"`
	Defaults      OrderedValues `json:"defaults"`
	Models        []ModelRow    `json:"models"`
	SkillsStatus  []SkillStatus `json:"skills_status"`
}

// Print computes --print's payload: every managed key's current resolved
// value and built-in default, the resolved per-role model/effort table, the
// skills manifest's installed status, and gentle-ai/engram/claude
// prerequisite detection. It never writes anything.
func Print(deps Deps, paths Paths) (PrintResult, error) {
	doc, exists, err := (Store{}).Load(paths.Config)
	if err != nil {
		return PrintResult{}, err
	}

	catalogue := config.Defaults()
	values := make(OrderedValues, 0, len(catalogue))
	defaults := make(OrderedValues, 0, len(catalogue))
	for _, kd := range catalogue {
		v, _ := config.ManagedValue(doc, kd.Key)
		values = append(values, OrderedEntry{Key: kd.Key, Value: v})
		defaults = append(defaults, OrderedEntry{Key: kd.Key, Value: kd.Value})
	}

	modelDefaults, err := models.PluginDefaults(deps.FS)
	if err != nil {
		return PrintResult{}, err
	}

	// A malformed or unreadable state.json is silently ignored: --print
	// degrades to unresolved from:<phase> display rather than failing the
	// whole call.
	phaseAssignments, phaseErr := gentleai.PhaseAssignments(paths.State)
	if phaseErr != nil {
		phaseAssignments = map[string]config.PhaseAssignment{}
	}

	modelTable := config.ModelTableFromDocument(doc, modelDefaults, phaseAssignments)
	roleInfo := config.Roles().Info
	modelRows := make([]ModelRow, len(modelTable))
	for i, r := range modelTable {
		info := roleInfo[r.Role]
		modelRows[i] = ModelRow{
			Role: r.Role, Model: r.Model, Effort: r.Effort, Source: r.Source,
			Purpose: info.Purpose, GentleAIEquivalent: info.GentleAIEquivalent, FromPhase: info.FromPhase,
		}
	}

	skillRows := []SkillStatus{}
	manifest, manifestErr := skills.LoadManifestFS(deps.FS)
	switch {
	case manifestErr == nil:
		for _, s := range skills.Status(manifest, paths.SkillsDir) {
			skillRows = append(skillRows, SkillStatus{
				Name: s.Name, Kind: s.Kind, Repo: s.Repo, Skill: s.Skill,
				Installed: s.Installed, Action: s.Action,
			})
		}
	case errors.Is(manifestErr, fs.ErrNotExist):
		// No manifest at all: skills_status stays empty.
	default:
		return PrintResult{}, manifestErr
	}

	prereq := gentleai.Prerequisites(context.Background(), deps.Runner, deps.LookPath)

	return PrintResult{
		ConfigPath: paths.Config,
		Exists:     exists,
		Prerequisites: Prerequisites{
			GentleAI: ToolVersion{Found: prereq.GentleAI.Found, Version: nonEmptyPtr(prereq.GentleAI.Version), OK: prereq.GentleAI.OK},
			Engram:   ToolFound{Found: prereq.Engram.Found},
			Claude:   ToolFound{Found: prereq.Claude.Found},
		},
		Values:       values,
		Defaults:     defaults,
		Models:       modelRows,
		SkillsStatus: skillRows,
	}, nil
}

func nonEmptyPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
