// Package releaseplan turns one project of the release: section into the
// pipeline `nerv release` runs in its build mode (compose or script), lists
// what the package stage left for the artifact, and holds the release-time
// file writers release needs (the MSBuild version files' version and release
// notes) and the coverage floor check.
package releaseplan
