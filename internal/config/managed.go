package config

import (
	"fmt"
	"regexp"
	"strings"
)

// ErrUnknownKey is returned by SetManagedValue for a key outside the
// managed catalogue. Its message matches configure.ps1's -Set rejection
// text exactly (key order and wording).
type ErrUnknownKey struct {
	Key string
}

func (e *ErrUnknownKey) Error() string {
	return fmt.Sprintf("Unknown config key '%s'. Managed keys: %s.", e.Key, strings.Join(ManagedKeys(), ", "))
}

// ErrInvalidValue is returned by SetManagedValue for a value outside a
// managed key's enumerated allowed values. Its message matches
// configure.ps1's -Set rejection text exactly.
type ErrInvalidValue struct {
	Key     string
	Value   string
	Allowed []string
}

func (e *ErrInvalidValue) Error() string {
	return fmt.Sprintf("Invalid value '%s' for key '%s'. Allowed: %s.", e.Value, e.Key, strings.Join(e.Allowed, ", "))
}

// ParseSetArg splits one "-Set"/"-SetModel"-style "key=value" argument on
// its first '=' into key and value. ok is false when there is no '=' at
// index >= 1 (a missing key, or no '=' at all). Mirrors
// ConvertFrom-NervSetArg.
func ParseSetArg(arg string) (key, value string, ok bool) {
	idx := strings.Index(arg, "=")
	if idx < 1 {
		return "", "", false
	}
	return arg[:idx], arg[idx+1:], true
}

// ValidateManagedKey reports an *ErrUnknownKey when key is not in the
// managed catalogue.
func ValidateManagedKey(key string) error {
	if _, ok := DefaultValue(key); !ok {
		return &ErrUnknownKey{Key: key}
	}
	return nil
}

// ValidateManagedValue reports an *ErrInvalidValue when key has an
// enumerated allowed-values set and value is not one of them. A key with no
// enumerated set (AllowedValues' ok == false) accepts any value.
func ValidateManagedValue(key, value string) error {
	allowed, ok := AllowedValues(key)
	if !ok {
		return nil
	}
	for _, a := range allowed {
		if a == value {
			return nil
		}
	}
	return &ErrInvalidValue{Key: key, Value: value, Allowed: allowed}
}

// ManagedValue reads one managed key's current resolved value out of doc,
// falling back to its Defaults() entry when the key (or its containing
// block) is absent. ok is false for a key not in the catalogue at all.
// Mirrors Get-NervManagedConfigValue.
func ManagedValue(doc *Document, key string) (value string, ok bool) {
	fallback, known := DefaultValue(key)
	if !known {
		return "", false
	}

	if strings.HasPrefix(key, "tasks.providers.teamwork.stages.") {
		stageName := strings.TrimPrefix(key, "tasks.providers.teamwork.stages.")
		stagesRaw, _ := doc.Scalar("tasks.providers.teamwork.stages")
		return stageDefault(stagesRaw, stageName, fallback), true
	}

	if strings.HasPrefix(key, "skills.") {
		category := strings.TrimPrefix(key, "skills.")
		return skillsCategoryDefault(doc, category, fallback), true
	}

	if key == "critical_paths" {
		raw, _ := doc.Scalar("critical_paths")
		if listValue := listDefaultFromScalar(raw); listValue != "" {
			return listValue, true
		}
		return fallback, true
	}

	if scalarValue, found := doc.Scalar(key); found && scalarValue != "" {
		return scalarValue, true
	}
	return fallback, true
}

// SetManagedValue validates key/value against the managed catalogue
// (ValidateManagedKey, ValidateManagedValue) and, when value differs from
// the key's current resolved value, writes it into doc — auto-vivifying
// any missing parent block, the same way the interactive wizard does.
// changed is false, with no document mutation, when validation fails or
// value already matches the current one. Mirrors
// Set-NervManagedConfigValue plus the validation configure.ps1's -Set
// handler performs before calling it.
func SetManagedValue(doc *Document, key, value string) (changed bool, err error) {
	if err := ValidateManagedKey(key); err != nil {
		return false, err
	}
	if err := ValidateManagedValue(key, value); err != nil {
		return false, err
	}

	oldValue, _ := ManagedValue(doc, key)
	if oldValue == value {
		return false, nil
	}

	writeManagedValue(doc, key, value)
	return true, nil
}

// writeManagedValue writes key's new value into doc without validating —
// callers must validate first. Mirrors Set-NervManagedConfigValue.
func writeManagedValue(doc *Document, key, value string) {
	if strings.HasPrefix(key, "tasks.providers.teamwork.stages.") {
		stageName := strings.TrimPrefix(key, "tasks.providers.teamwork.stages.")
		stagesRaw, _ := doc.Scalar("tasks.providers.teamwork.stages")
		order := []string{"inDev", "testing", "implemented", "blocked", "canceled", "pending", "analysis"}
		parts := make([]string, 0, len(order))
		for _, stage := range order {
			var current string
			if stage == stageName {
				current = value
			} else {
				fallback, _ := DefaultValue("tasks.providers.teamwork.stages." + stage)
				current = stageDefault(stagesRaw, stage, fallback)
			}
			parts = append(parts, stage+": "+current)
		}
		doc.SetScalar("tasks.providers.teamwork.stages", "{ "+strings.Join(parts, ", ")+" }", Raw())
		return
	}

	if strings.HasPrefix(key, "skills.") {
		category := strings.TrimPrefix(key, "skills.")

		if !doc.KeyExists("skills") {
			categories := []string{"testing", "code", "best-practices", "architecture", "audit"}
			values := SkillsValues{}
			for _, cat := range categories {
				var v string
				if cat == category {
					v = value
				} else {
					fallback, _ := DefaultValue("skills." + cat)
					v = skillsCategoryDefault(doc, cat, fallback)
				}
				switch cat {
				case "testing":
					values.Testing = v
				case "code":
					values.Code = v
				case "best-practices":
					values.BestPractices = v
				case "architecture":
					values.Architecture = v
				case "audit":
					values.Audit = v
				}
			}
			doc.SetBlock("skills", FormatSkillsBlock(values))
			return
		}

		if doc.KeyIsInline("skills") {
			currentInline, _ := doc.Scalar("skills")
			currentInline = SetInlineListField(currentInline, category, value)
			doc.SetScalar("skills", currentInline, Raw())
			return
		}

		doc.SetScalar("skills."+category, "["+value+"]", Raw())
		return
	}

	if key == "critical_paths" {
		list := splitTrim(value, ",")
		if !doc.KeyExists("critical_paths") {
			doc.SetBlock("critical_paths", FormatCriticalPathsLine(list))
			return
		}
		doc.SetScalar("critical_paths", "["+strings.Join(list, ", ")+"]", Raw())
		return
	}

	doc.SetScalar(key, value)
}

// ---------------------------------------------------------------------------
// Small helpers ported from Get-NervListDefaultFromScalar,
// Get-NervStageDefault, Get-NervSkillsCategoryDefault.
// ---------------------------------------------------------------------------

// listDefaultFromScalar strips a scalar's optional "[ ... ]" bracket
// wrapper and trims it. Mirrors Get-NervListDefaultFromScalar.
func listDefaultFromScalar(raw string) string {
	v := strings.TrimSpace(raw)
	if v == "" {
		return ""
	}
	if strings.HasPrefix(v, "[") && strings.HasSuffix(v, "]") {
		v = v[1 : len(v)-1]
	}
	return strings.TrimSpace(v)
}

var stageValueRe = regexp.MustCompile(`:\s*([^,}]+)`)

// stageDefault reads one stage name's current value out of a raw
// "{ inDev: ..., testing: ..., ... }" stages inline-map text, or fallback
// when stagesRaw is empty or the stage isn't found. Mirrors
// Get-NervStageDefault.
func stageDefault(stagesRaw, stageKey, fallback string) string {
	if stagesRaw == "" {
		return fallback
	}
	pattern := regexp.MustCompile(regexp.QuoteMeta(stageKey) + stageValueRe.String())
	if m := pattern.FindStringSubmatch(stagesRaw); m != nil {
		return strings.TrimSpace(m[1])
	}
	return fallback
}

// skillsCategoryDefault reads one skills category's current
// comma-separated list, whether skills: is written as a block or as an
// inline map, falling back to fallback when the category is not found in
// either form (or skills: does not exist at all). Mirrors
// Get-NervSkillsCategoryDefault.
func skillsCategoryDefault(doc *Document, category, fallback string) string {
	if !doc.KeyExists("skills") {
		return fallback
	}

	if doc.KeyIsInline("skills") {
		inlineRaw, _ := doc.Scalar("skills")
		if val, ok := InlineListField(inlineRaw, category); ok && val != "" {
			return val
		}
		return fallback
	}

	raw, _ := doc.Scalar("skills." + category)
	if val := listDefaultFromScalar(raw); val != "" {
		return val
	}
	return fallback
}

// splitTrim splits value on sep, trims each part, and drops empty parts.
// Mirrors the critical_paths $Value -split ',' | Trim | Where-Object
// pipeline in Set-NervManagedConfigValue.
func splitTrim(value, sep string) []string {
	parts := strings.Split(value, sep)
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			result = append(result, t)
		}
	}
	return result
}
