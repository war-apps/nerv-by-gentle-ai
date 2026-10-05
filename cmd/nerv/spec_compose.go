package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"

	"github.com/war-apps/nerv-by-gentle-ai/internal/atomicfile"
	"github.com/war-apps/nerv-by-gentle-ai/internal/refusal"
	"github.com/war-apps/nerv-by-gentle-ai/internal/specs"
)

const specComposeUsage = `Usage: nerv spec-compose --canonical <path> --delta <path> [--output <path|->]

Merges a change's delta spec into the canonical spec it amends, applying
RENAMED, MODIFIED, REMOVED, then ADDED requirements in that order and
preserving every unrelated byte.

Flags:
  --canonical <path>   The canonical openspec/specs/<domain>/spec.md
  --delta <path>       The change's openspec/changes/<change>/specs/<domain>/spec.md
  --output <path|->    Where to write the composed spec (default "-", stdout).
                       A file is replaced atomically, keeping its mode.

On an unapplied delta, nothing is written: the error names the section and
requirement on stderr and the exit code is 1.
`

// runSpecCompose is "nerv spec-compose"'s CLI: it reads the two specs,
// composes them, and writes the result only when the whole composition
// succeeded. A delta that cannot be applied is a refusal (exit 1); a usage
// error or an unexpected I/O failure exits 2.
func runSpecCompose(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("spec-compose", flag.ContinueOnError)
	flags.SetOutput(io.Discard)

	canonicalPath := flags.String("canonical", "", "")
	deltaPath := flags.String("delta", "", "")
	outputPath := flags.String("output", "-", "")

	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(stdout, specComposeUsage)
			return 0
		}
		fmt.Fprint(stderr, specComposeUsage)
		return 2
	}
	if flags.NArg() != 0 || *canonicalPath == "" || *deltaPath == "" {
		fmt.Fprint(stderr, specComposeUsage)
		return 2
	}

	composed, err := composeSpecFiles(*canonicalPath, *deltaPath)
	if err == nil {
		err = writeComposedSpec(*outputPath, composed, stdout)
	}
	if err != nil {
		fmt.Fprintf(stderr, "nerv: %v\n", err)
		return exitCodeFor(err)
	}
	return 0
}

func composeSpecFiles(canonicalPath, deltaPath string) (string, error) {
	canonical, err := readSpecFile("canonical", canonicalPath)
	if err != nil {
		return "", err
	}
	delta, err := readSpecFile("delta", deltaPath)
	if err != nil {
		return "", err
	}
	composed, err := specs.Compose(canonical, delta)
	if err != nil {
		return "", &refusal.Error{Err: err}
	}
	return composed, nil
}

// readSpecFile reads one input; a missing file is a refusal (bad argument),
// any other read failure an environment error.
func readSpecFile(label, path string) (string, error) {
	data, err := os.ReadFile(path)
	if err == nil {
		return string(data), nil
	}
	err = fmt.Errorf("read %s spec: %w", label, err)
	if errors.Is(err, fs.ErrNotExist) {
		return "", &refusal.Error{Err: err}
	}
	return "", err
}

func writeComposedSpec(output, composed string, stdout io.Writer) error {
	if output == "-" {
		_, err := io.WriteString(stdout, composed)
		return err
	}
	if err := atomicfile.Write(output, []byte(composed), 0o644); err != nil {
		return fmt.Errorf("write composed spec: %w", err)
	}
	return nil
}
