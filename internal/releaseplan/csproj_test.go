package releaseplan

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

const bom = "\xef\xbb\xbf"

// writeTemp writes content to a .csproj in a temp dir and returns its path.
func writeTemp(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "Api.csproj")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestWriteCsproj(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		version string
		notes   string
		want    string
	}{
		{
			name: "existing elements are replaced, the rest is untouched",
			in: `<Project Sdk="Microsoft.NET.Sdk.Web">
  <PropertyGroup>
    <TargetFramework>net10.0</TargetFramework>
    <Version>2.8.0</Version>
    <PackageReleaseNotes>https://example.test/releases/tag/v2.8.0</PackageReleaseNotes>
    <VersionPrefix>9.9.9</VersionPrefix>
  </PropertyGroup>
</Project>
`,
			version: "2.9.0-alpha.1",
			notes:   "https://example.test/releases/tag/v2.9.0-alpha.1",
			want: `<Project Sdk="Microsoft.NET.Sdk.Web">
  <PropertyGroup>
    <TargetFramework>net10.0</TargetFramework>
    <Version>2.9.0-alpha.1</Version>
    <PackageReleaseNotes>https://example.test/releases/tag/v2.9.0-alpha.1</PackageReleaseNotes>
    <VersionPrefix>9.9.9</VersionPrefix>
  </PropertyGroup>
</Project>
`,
		},
		{
			name: "BOM and CRLF kept, missing elements added to the first unconditioned group",
			in: bom + "<Project Sdk=\"Microsoft.NET.Sdk\">\r\n" +
				"  <PropertyGroup Condition=\"'$(Configuration)' == 'Debug'\">\r\n" +
				"    <DebugType>full</DebugType>\r\n" +
				"  </PropertyGroup>\r\n" +
				"  <PropertyGroup Label=\"Package\">\r\n" +
				"\t\t<TargetFramework>net10.0</TargetFramework>\r\n" +
				"  </PropertyGroup>\r\n" +
				"</Project>\r\n",
			version: "1.0.0",
			notes:   "line one\nline two",
			want: bom + "<Project Sdk=\"Microsoft.NET.Sdk\">\r\n" +
				"  <PropertyGroup Condition=\"'$(Configuration)' == 'Debug'\">\r\n" +
				"    <DebugType>full</DebugType>\r\n" +
				"  </PropertyGroup>\r\n" +
				"  <PropertyGroup Label=\"Package\">\r\n" +
				"\t\t<Version>1.0.0</Version>\r\n" +
				"\t\t<PackageReleaseNotes>line one\r\nline two</PackageReleaseNotes>\r\n" +
				"\t\t<TargetFramework>net10.0</TargetFramework>\r\n" +
				"  </PropertyGroup>\r\n" +
				"</Project>\r\n",
		},
		{
			name: "only the missing element is added",
			in: `<Project>
  <PropertyGroup>
    <Version>1.0.0</Version>
  </PropertyGroup>
</Project>`,
			version: "1.1.0",
			notes:   "n",
			want: `<Project>
  <PropertyGroup>
    <PackageReleaseNotes>n</PackageReleaseNotes>
    <Version>1.1.0</Version>
  </PropertyGroup>
</Project>`,
		},
		{
			name: "notes are XML and MSBuild escaped",
			in: `<Project>
  <PropertyGroup>
    <PackageReleaseNotes><![CDATA[old]]></PackageReleaseNotes>
    <Version />
  </PropertyGroup>
</Project>
`,
			version: "3.0.0",
			notes:   "* fix a < b & c > d\n* use $(Foo) and @(Items) at 50%; \"quoted\" 'single'\r\n",
			want: `<Project>
  <PropertyGroup>
    <PackageReleaseNotes>* fix a &lt; b &amp; c &gt; d
* use %24(Foo) and %40(Items) at 50%25; "quoted" 'single'
</PackageReleaseNotes>
    <Version>3.0.0</Version>
  </PropertyGroup>
</Project>
`,
		},
		{
			name: "no unconditioned group: a new one after the Project tag",
			in: `<Project Sdk="Microsoft.NET.Sdk">
  <PropertyGroup Condition="'$(X)' == ''">
    <A>1</A>
  </PropertyGroup>
  <PropertyGroup />
</Project>
`,
			version: "1.0.0",
			notes:   "",
			want: `<Project Sdk="Microsoft.NET.Sdk">
  <PropertyGroup>
    <Version>1.0.0</Version>
    <PackageReleaseNotes></PackageReleaseNotes>
  </PropertyGroup>
  <PropertyGroup Condition="'$(X)' == ''">
    <A>1</A>
  </PropertyGroup>
  <PropertyGroup />
</Project>
`,
		},
		{
			name: "elements inside comments are left alone",
			in: `<Project>
  <!-- <Version>0.0.1</Version> -->
  <PropertyGroup>
    <Version>1.0.0</Version>
    <PackageReleaseNotes>x</PackageReleaseNotes>
  </PropertyGroup>
</Project>
`,
			version: "1.0.1",
			notes:   "y",
			want: `<Project>
  <!-- <Version>0.0.1</Version> -->
  <PropertyGroup>
    <Version>1.0.1</Version>
    <PackageReleaseNotes>y</PackageReleaseNotes>
  </PropertyGroup>
</Project>
`,
		},
		{
			name: "an empty group gets the elements one level deeper",
			in: `<Project>
    <PropertyGroup></PropertyGroup>
</Project>
`,
			version: "1.0.0",
			notes:   "z",
			want: `<Project>
    <PropertyGroup>
      <Version>1.0.0</Version>
      <PackageReleaseNotes>z</PackageReleaseNotes></PropertyGroup>
</Project>
`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := writeTemp(t, tt.in)
			if err := WriteCsproj(p, tt.version, tt.notes); err != nil {
				t.Fatalf("WriteCsproj = %v", err)
			}
			if got := readFile(t, p); got != tt.want {
				t.Errorf("file =\n%q\nwant\n%q", got, tt.want)
			}
			// Writing the same values again changes nothing.
			if err := WriteCsproj(p, tt.version, tt.notes); err != nil {
				t.Fatalf("second WriteCsproj = %v", err)
			}
			if got := readFile(t, p); got != tt.want {
				t.Errorf("second write changed the file:\n%q", got)
			}
		})
	}
}

func TestWriteCsproj_Errors(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		version string
		want    string
	}{
		{"invalid version", "<Project><PropertyGroup></PropertyGroup></Project>", "1.0.0</Version><X>", "invalid version"},
		{"empty version", "<Project><PropertyGroup></PropertyGroup></Project>", "", "invalid version"},
		{"not a project file", "<Nope/>", "1.0.0", "no <Project> element"},
		{"unclosed element", "<Project><PropertyGroup><Version>1.0.0</PropertyGroup></Project>", "1.0.1", "<Version> is not closed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := writeTemp(t, tt.in)
			err := WriteCsproj(p, tt.version, "n")
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("WriteCsproj = %v, want an error containing %q", err, tt.want)
			}
			if got := readFile(t, p); got != tt.in {
				t.Errorf("file changed on error: %q", got)
			}
		})
	}
	if err := WriteCsproj(filepath.Join(t.TempDir(), "missing.csproj"), "1.0.0", ""); err == nil {
		t.Error("missing file: want an error")
	}
}

// failingTemp is a temp file whose step named fail returns an error; it
// records the calls it got.
type failingTemp struct {
	tempFile
	fail  string
	calls *[]string
}

func (f failingTemp) step(name string, real func() error) error {
	*f.calls = append(*f.calls, name)
	if f.fail == name {
		return errors.New("injected " + name + " failure")
	}
	return real()
}

func (f failingTemp) Write(p []byte) (int, error) {
	if err := f.step("write", func() error { return nil }); err != nil {
		f.tempFile.Write(p[:len(p)/2]) // a partial write
		return len(p) / 2, err
	}
	return f.tempFile.Write(p)
}
func (f failingTemp) Sync() error { return f.step("sync", f.tempFile.Sync) }

// Close always closes the real file, then reports the injected failure:
// a failed close still releases the descriptor, as os.File does.
func (f failingTemp) Close() error {
	realErr := f.tempFile.Close()
	return f.step("close", func() error { return realErr })
}

const smallProject = "<Project>\n  <PropertyGroup>\n    <Version>1.0.0</Version>\n    <PackageReleaseNotes>a</PackageReleaseNotes>\n  </PropertyGroup>\n</Project>\n"

func TestWriteCsproj_AtomicReplace(t *testing.T) {
	for _, fail := range []string{"write", "sync", "close", ""} {
		t.Run("failure at "+fail, func(t *testing.T) {
			var calls []string
			var real *os.File
			defer func(orig func(string, string) (tempFile, error)) { createTemp = orig }(createTemp)
			createTemp = func(dir, pattern string) (tempFile, error) {
				f, err := os.CreateTemp(dir, pattern)
				real = f
				return failingTemp{tempFile: f, fail: fail, calls: &calls}, err
			}
			p := writeTemp(t, smallProject)
			if err := os.Chmod(p, 0o640); err != nil {
				t.Fatal(err)
			}
			err := WriteCsproj(p, "2.0.0", "b")
			entries, _ := os.ReadDir(filepath.Dir(p))
			if len(entries) != 1 {
				t.Errorf("leftover files next to the project: %v", entries)
			}
			got := readFile(t, p)
			if fail != "" {
				if err == nil || !strings.Contains(err.Error(), "injected "+fail) {
					t.Fatalf("WriteCsproj = %v, want the injected failure", err)
				}
				if got != smallProject {
					t.Errorf("the original changed on failure: %q", got)
				}
				// The descriptor is closed whatever failed (Windows cannot
				// remove an open file).
				if _, err := real.Write([]byte("x")); !errors.Is(err, os.ErrClosed) {
					t.Errorf("the temp file is still open after a %s failure: %v", fail, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(got, "<Version>2.0.0</Version>") {
				t.Errorf("file = %q", got)
			}
			if want := []string{"write", "sync", "close"}; !slices.Equal(calls, want) {
				t.Errorf("calls = %v, want %v (synced before the rename)", calls, want)
			}
			if info, _ := os.Stat(p); runtime.GOOS != "windows" && info.Mode().Perm() != 0o640 {
				t.Errorf("mode = %v, want the original 0640", info.Mode().Perm())
			}
		})
	}
}

func TestWriteCsproj_ThroughASymlinkKeepsTheLink(t *testing.T) {
	p := writeTemp(t, smallProject)
	link := filepath.Join(t.TempDir(), "link.csproj")
	if err := os.Symlink(p, link); err != nil {
		t.Skip("no symlinks:", err)
	}
	if err := WriteCsproj(link, "2.0.0", "b"); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("the link was replaced by a file")
	}
	if !strings.Contains(readFile(t, p), "<Version>2.0.0</Version>") {
		t.Error("the target was not updated")
	}
}
