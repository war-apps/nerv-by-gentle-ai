# Releases

**Versioning.** The plugin follows [SemVer](https://semver.org/). The
`version` in `plugin/.claude-plugin/plugin.json`, the git tag `vX.Y.Z`,
the matching `## [X.Y.Z]` section in `CHANGELOG.md`, and the `nerv version`
binary version (injected at build time from the tag by goreleaser) all
agree. This is not cosmetic: Claude Code detects plugin updates by
comparing `plugin.json`'s `version` string, so a release that does not
bump it is invisible to every installed user; `nerv release guard` asserts
the binary and plugin versions agree before a release is allowed to ship.

**How the version is computed.** `nerv release preview`/`apply` derives the
next version from the [Conventional Commits](https://www.conventionalcommits.org/)
reachable since the last `vX.Y.Z` tag: `feat` bumps minor, `fix`/`perf`
bump patch, a `!` before the colon or a `BREAKING CHANGE:` footer bumps
major, and every other type (`docs`, `chore`, `test`, `refactor`, `ci`,
`build`, `style`) contributes no bump on its own.

**Channels.** Every push to a Gitflow branch publishes automatically —
nothing to run by hand:

| Branch push | Publishes |
|---|---|
| `develop` | `vX.Y.Z-alpha.N` pre-release — version computed from Conventional Commits since the last stable tag; nothing is published when there is nothing releasable. |
| `release/*` | `vX.Y.Z-rc.N` pre-release — version taken from `plugin.json` on that branch. |
| `main` (merge) | Stable `vX.Y.Z` release — the matching `CHANGELOG.md` section becomes the release notes, and goreleaser publishes the linux/darwin amd64/arm64 binaries and their checksums. |

**Cutting a release (Gitflow).**

1. `git checkout -b release/X.Y.Z develop`
2. `go run ./cmd/nerv release preview` — review the computed version and
   changelog section.
3. `go run ./cmd/nerv release apply` (or `--version X.Y.Z` to override the
   computed bump) — writes `plugin.json` and inserts the section into
   `CHANGELOG.md`.
4. Review `CHANGELOG.md`, then commit `chore(release): X.Y.Z`.
5. Open the PR to `main`. On merge, the Release workflow runs the full
   suite plus `nerv release guard`, creates the tag and the GitHub Release
   with that changelog section as its notes, runs goreleaser to publish
   the binaries/checksums/formula, and opens the back-merge PR to
   `develop` — merge that PR to close the loop.

Hotfixes follow the same steps from `hotfix/X.Y.Z` branched off `main`
instead of `develop`.

**Pre-releases.** Pre-releases are automatic (see "Channels" above): the
`release.yml` workflow itself computes the version, creates the tag, and
publishes the GitHub pre-release when it sees the push — a `release/*`
push runs the `rc` job and a `develop` push runs the `alpha` job, neither
going through `main` nor through goreleaser (goreleaser only runs on the
stable release, after the tag is created). There is nothing to run by hand
and no tag to push yourself; `nerv release preview --pre-release
alpha|rc` is what the workflow calls internally.

**What the guard refuses, and how to recover.** `nerv release guard` fails
the Release workflow before it tags or publishes anything when the version
in `plugin.json` is already tagged, `CHANGELOG.md` has no matching
`## [X.Y.Z]` section, or the binary version and the embedded plugin
version disagree. If the Release job fails *after* the tag was already
pushed (e.g. the GitHub Release step itself failed), create the release by
hand with `gh release create vX.Y.Z --notes-file <section>` — re-running
the workflow will refuse, since the guard sees the tag already exists.

**CI.** `.github/workflows/ci.yml` runs `gofmt -l`, `go vet ./...`, and
`go test ./...`, plus the two hook suites
(`tests/hook-session-start.test.sh`, `tests/hook-engram-project.test.sh`)
and `tests/install-sh.test.sh`, on every pull request targeting `develop`
or `main`.

**Marketplace consumers.** A marketplace added from GitHub serves the
repository's default branch. For users to receive released versions
rather than in-progress `develop` content, the default branch must be
`main`, or the marketplace entry must pin an explicit `ref`.
