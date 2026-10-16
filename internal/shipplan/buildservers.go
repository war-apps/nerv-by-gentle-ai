package shipplan

// DisableBuildServers makes dotnet restore, build, test and pack run without
// the MSBuild and compiler servers. dotnet format rejects it (SDK 10:
// "unrecognized command or argument"), so it is never passed there.
const DisableBuildServers = "--disable-build-servers"

// DotnetBuildServerEnv is the environment every dotnet command NERV builds
// gets, with DisableBuildServers where accepted: no MSBuild node reuse, no
// shared compilation (VBCSCompiler), no MSBuild server. On shared hosts (the
// team's T2) reused servers made dotnet die mid-release with "Internal CLR
// error (0x80131506)". A repository's explicit env for the same command
// (pipeline.dotnet.env) wins; user stage overrides are not rewritten. A
// fresh map on every call.
func DotnetBuildServerEnv() map[string]string {
	return map[string]string{
		"MSBUILDDISABLENODEREUSE":       "1",
		"UseSharedCompilation":          "false",
		"DOTNET_CLI_USE_MSBUILD_SERVER": "0",
	}
}
