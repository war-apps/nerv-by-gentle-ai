package shipplan

import (
	"os"
	"path/filepath"
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
