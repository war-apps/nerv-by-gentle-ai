package shipplan

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// versionRe is what WriteCsproj accepts as a version: a semantic version's
// characters, nothing XML or MSBuild would read specially.
var versionRe = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z.+-]*$`)

// WriteCsproj sets <Version> and <PackageReleaseNotes> in the MSBuild
// project file at path, the dotnet preset's version write (T6 runs it at
// semantic-release's prepare step, so the file enters the release commit;
// modelled on dim.comprobantes.api's scripts/bump-csproj.sh, but as a text
// edit instead of sed).
//
// Every existing element of each name outside XML comments gets the new
// value (its attributes, e.g. a Condition, are kept; a self-closing
// <Version /> is opened). A missing element is added at the top of the
// first <PropertyGroup> without a Condition attribute, indented like that
// group's first child; when there is no such group, a new one is added
// right after the <Project> tag. Everything else stays byte for byte,
// including a UTF-8 BOM and the file's line endings (the notes are written
// with them too).
//
// The notes are XML-escaped (&, <, >) and MSBuild-escaped (% as %25, $ as
// %24, @ as %40), so a "$(Foo)" in a commit message is not expanded by
// MSBuild; dotnet pack unescapes them. The version must be a semantic
// version's characters ([0-9A-Za-z.+-]). The file is replaced atomically
// (a synced temp file renamed over it, keeping its permission bits), so on
// any error it is left byte for byte unchanged.
func WriteCsproj(path, version, releaseNotes string) error {
	if !versionRe.MatchString(version) {
		return fmt.Errorf("%s: invalid version %q", path, version)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	out, err := setCsprojProperties(string(data), []property{
		{"Version", version},
		{"PackageReleaseNotes", escapeNotes(releaseNotes, lineEnding(string(data)))},
	})
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	if out == string(data) {
		return nil
	}
	if err := replaceFile(path, []byte(out)); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

// replaceFile replaces the file at path (through a symlink, its target)
// with data atomically: a temp file in the same directory with the
// original's permission bits, written and synced, then renamed over it. On
// any error the original is untouched and the temp file removed.
func replaceFile(path string, data []byte) (err error) {
	if resolved, rerr := filepath.EvalSymlinks(path); rerr == nil {
		path = resolved
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	tmp, err := createTemp(filepath.Dir(path), "."+filepath.Base(path)+".nerv-*.tmp")
	if err != nil {
		return err
	}
	closed := false
	defer func() {
		if err != nil {
			if !closed {
				tmp.Close()
			}
			os.Remove(tmp.Name())
		}
	}()
	if err = tmp.Chmod(info.Mode().Perm()); err != nil {
		return err
	}
	if _, err = tmp.Write(data); err != nil {
		return err
	}
	if err = tmp.Sync(); err != nil {
		return err
	}
	closed = true
	if err = tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// property is one MSBuild property to set; value is already escaped.
type property struct{ name, value string }

// lineEnding is the file's line ending: CRLF when it holds one, else LF.
func lineEnding(s string) string {
	if strings.Contains(s, "\r\n") {
		return "\r\n"
	}
	return "\n"
}

// escapeNotes escapes release notes for an MSBuild property element and
// writes their lines with eol.
func escapeNotes(notes, eol string) string {
	notes = strings.ReplaceAll(notes, "\r\n", "\n")
	r := strings.NewReplacer(
		"&", "&amp;", "<", "&lt;", ">", "&gt;",
		"%", "%25", "$", "%24", "@", "%40",
	)
	return strings.ReplaceAll(r.Replace(notes), "\n", eol)
}

// setCsprojProperties returns text with each property set, as described on
// WriteCsproj.
func setCsprojProperties(text string, props []property) (string, error) {
	if _, ok := findTag(text, "Project", 0); !ok {
		return "", errors.New("no <Project> element: not an MSBuild project file")
	}
	var missing []property
	for _, p := range props {
		out, found, err := replaceElements(text, p)
		if err != nil {
			return "", err
		}
		text = out
		if !found {
			missing = append(missing, p)
		}
	}
	if len(missing) == 0 {
		return text, nil
	}
	return insertProperties(text, missing), nil
}

// tag is an opening tag found in a text: text[start:end] is "<Name ...>".
type tag struct {
	start, end  int
	attrs       string // between the name and ">" or "/>"
	selfClosing bool
}

// findTag finds the next opening tag <name ...> at or after from, outside
// XML comments. A name followed by more name characters (<VersionPrefix>
// for Version) is not a match.
func findTag(text, name string, from int) (tag, bool) {
	open := "<" + name
	for i := from; ; {
		j := strings.Index(text[i:], open)
		if j < 0 {
			return tag{}, false
		}
		start := i + j
		if c := commentEnd(text, start); c > 0 {
			i = c
			continue
		}
		after := start + len(open)
		if after >= len(text) || !strings.ContainsRune(" \t\r\n/>", rune(text[after])) {
			i = after
			continue
		}
		k := strings.IndexByte(text[after:], '>')
		if k < 0 {
			return tag{}, false
		}
		end := after + k + 1
		attrs := text[after : end-1]
		t := tag{start: start, end: end, attrs: attrs}
		if strings.HasSuffix(attrs, "/") {
			t.selfClosing = true
			t.attrs = strings.TrimSuffix(attrs, "/")
		}
		return t, true
	}
}

// commentEnd returns the end of the XML comment holding position pos, or 0
// when pos is outside every comment.
func commentEnd(text string, pos int) int {
	for i := 0; ; {
		j := strings.Index(text[i:], "<!--")
		if j < 0 || i+j > pos {
			return 0
		}
		start := i + j
		k := strings.Index(text[start+4:], "-->")
		if k < 0 {
			return len(text) // an unclosed comment runs to the end
		}
		end := start + 4 + k + 3
		if pos < end {
			return end
		}
		i = end
	}
}

// replaceElements sets the content of every <name> element of text to the
// property's value and reports whether there was one.
func replaceElements(text string, p property) (string, bool, error) {
	var b strings.Builder
	found := false
	last := 0
	for from := 0; ; {
		t, ok := findTag(text, p.name, from)
		if !ok {
			break
		}
		found = true
		if t.selfClosing {
			b.WriteString(text[last:t.start])
			fmt.Fprintf(&b, "<%s%s>%s</%s>", p.name, strings.TrimRight(t.attrs, " \t\r\n"), p.value, p.name)
			last, from = t.end, t.end
			continue
		}
		closeAt := strings.Index(text[t.end:], "</"+p.name)
		if closeAt < 0 {
			return "", false, fmt.Errorf("<%s> is not closed", p.name)
		}
		closeAt += t.end
		b.WriteString(text[last:t.end])
		b.WriteString(p.value)
		last, from = closeAt, closeAt
	}
	b.WriteString(text[last:])
	return b.String(), found, nil
}

// insertProperties adds props at the top of the first unconditioned
// <PropertyGroup>, or in a new group after the <Project> tag.
func insertProperties(text string, props []property) string {
	eol := lineEnding(text)
	elements := func(indent string) string {
		var b strings.Builder
		for _, p := range props {
			fmt.Fprintf(&b, "%s%s<%s>%s</%s>", eol, indent, p.name, p.value, p.name)
		}
		return b.String()
	}
	for from := 0; ; {
		t, ok := findTag(text, "PropertyGroup", from)
		if !ok {
			break
		}
		from = t.end
		if t.selfClosing || strings.Contains(t.attrs, "Condition") {
			continue
		}
		indent := childIndent(text, t)
		return text[:t.end] + elements(indent) + text[t.end:]
	}
	project, _ := findTag(text, "Project", 0)
	unit := "  "
	group := eol + unit + "<PropertyGroup>" + elements(unit+unit) + eol + unit + "</PropertyGroup>"
	return text[:project.end] + group + text[project.end:]
}

// childIndent is the indentation of the first child line of the group
// opened by t, or the group's own indentation plus two spaces.
func childIndent(text string, t tag) string {
	rest := text[t.end:]
	rest = strings.TrimPrefix(rest, "\r")
	if strings.HasPrefix(rest, "\n") {
		line := rest[1:]
		ws := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
		if after := line[len(ws):]; strings.HasPrefix(after, "<") && !strings.HasPrefix(after, "</") {
			return ws
		}
	}
	return lineIndent(text, t.start) + "  "
}

// lineIndent is the leading whitespace of the line holding pos.
func lineIndent(text string, pos int) string {
	lineStart := strings.LastIndexByte(text[:pos], '\n') + 1
	line := text[lineStart:pos]
	return line[:len(line)-len(strings.TrimLeft(line, " \t"))]
}

// tempFile is the temp file WriteCsproj writes before renaming it over the
// project file.
type tempFile interface {
	Write([]byte) (int, error)
	Sync() error
	Close() error
	Name() string
	Chmod(os.FileMode) error
}

// createTemp creates the temp file; a variable so tests can inject
// failures.
var createTemp = func(dir, pattern string) (tempFile, error) { return os.CreateTemp(dir, pattern) }
