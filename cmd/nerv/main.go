// Command nerv is the NERV Gentle-AI CLI: it materializes the embedded
// plugin tree and reports version information.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"

	nerv "github.com/war-apps/nerv-gentle-ai"
	"github.com/war-apps/nerv-gentle-ai/internal/version"
)

const usage = `Usage: nerv <command> [flags]

Commands:
  version [--json]   Print the binary and plugin versions
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run dispatches the given command-line arguments and returns the process
// exit code. It never touches package-level state so it can be exercised
// directly from tests.
func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stdout, usage)
		return 0
	}

	switch args[0] {
	case "version":
		return runVersion(args[1:], stdout, stderr)
	default:
		fmt.Fprint(stderr, usage)
		return 2
	}
}

func runVersion(args []string, stdout, stderr io.Writer) int {
	jsonOutput := false
	for _, arg := range args {
		if arg == "--json" {
			jsonOutput = true
			continue
		}
		fmt.Fprint(stderr, usage)
		return 2
	}

	pluginVersion, err := version.PluginVersion(pluginFS())
	if err != nil {
		fmt.Fprintf(stderr, "nerv: %v\n", err)
		return 2
	}

	if jsonOutput {
		info := version.Info{Binary: version.Binary, Plugin: pluginVersion}
		encoded, err := json.Marshal(info)
		if err != nil {
			fmt.Fprintf(stderr, "nerv: %v\n", err)
			return 2
		}
		fmt.Fprintf(stdout, "%s\n", encoded)
		return 0
	}

	fmt.Fprintf(stdout, "nerv %s (plugin %s)\n", version.Binary, pluginVersion)
	return 0
}

func pluginFS() fs.FS {
	return nerv.PluginFS()
}
