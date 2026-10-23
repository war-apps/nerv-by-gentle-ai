package nerv

import (
	"io/fs"
	"strings"
	"testing"
)

// S11 / L8: the user config is os.UserConfigDir()/ai-workflow/config.yml.
// Prose that tells an agent to read it must name the real resolution.
func TestS11_ProseResolvesUserConfigPerPlatform(t *testing.T) {
	for name, fsys := range map[string]fs.FS{"plugin": PluginFS(), "plugins": ExtensionsFS()} {
		_ = fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".md") {
				return err
			}
			data, _ := fs.ReadFile(fsys, path)
			text := string(data)
			if !strings.Contains(text, "~/.config/ai-workflow/config.yml") {
				return nil
			}
			if !strings.Contains(text, "config_path") && !strings.Contains(text, "%AppData%") {
				t.Errorf("%s/%s hard-wires ~/.config/ai-workflow/config.yml: on Windows, macOS or with XDG_CONFIG_HOME, nerv configure writes elsewhere", name, path)
			}
			return nil
		})
	}
}
