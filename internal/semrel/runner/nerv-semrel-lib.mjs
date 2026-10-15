// nerv-semrel-lib.mjs: the pure logic of nerv-semrel.mjs (no semantic-release,
// no process state), embedded next to it and unit tested by
// nerv-semrel-lib.test.mjs.

export const RESULT_MARKER = "NERV_SEMREL_RESULT ";

// finish writes the result line and exits with code only once stdout has
// taken it. On a pipe, stdout writes are asynchronous: exiting right after a
// large write (long release notes) drops whatever the pipe did not take yet,
// observed at 64 KiB. The explicit exit stays because semantic-release and
// its plugins can leave handles that would keep the process alive.
export function finish(stream, result, code, exit = (c) => process.exit(c)) {
  stream.write(`\n${RESULT_MARKER}${JSON.stringify(result)}\n`, () => exit(code));
}

export const STEPS = [
  "verifyConditions",
  "analyzeCommits",
  "verifyRelease",
  "generateNotes",
  "prepare",
  "publish",
  "addChannel",
  "success",
  "fail",
];

// semantic-release's default plugins (lib/get-config.js).
export const DEFAULT_PLUGINS = [
  "@semantic-release/commit-analyzer",
  "@semantic-release/release-notes-generator",
  "@semantic-release/npm",
  "@semantic-release/github",
];

// isPackage tells a bare npm specifier from a file path.
export function isPackage(name) {
  return typeof name === "string" && name !== "" && !/^(\.|\/|[A-Za-z]:[\\/]|\\\\|file:)/.test(name);
}

// packageName drops a subpath: "@scope/pkg/x" -> "@scope/pkg", "pkg/x" -> "pkg".
export function packageName(spec) {
  const parts = spec.split("/");
  return spec.startsWith("@") ? parts.slice(0, 2).join("/") : parts[0];
}

// pluginRefs lists [name, config] for every plugin reference in a plugins
// list or a step value (string, [name, config], {path, ...config}).
export function pluginRefs(value) {
  const out = [];
  const one = (p) => {
    if (typeof p === "string") out.push([p, {}]);
    else if (Array.isArray(p)) out.push([p[0], p[1] || {}]);
    else if (p && typeof p === "object" && typeof p.path === "string") {
      const { path: name, ...config } = p;
      out.push([name, config]);
    }
  };
  // A step value is either one reference or a list of them; a two-element
  // [string, plain config] array is one reference with its configuration.
  const single =
    Array.isArray(value) &&
    value.length === 2 &&
    typeof value[0] === "string" &&
    value[1] !== null &&
    typeof value[1] === "object" &&
    !Array.isArray(value[1]) &&
    typeof value[1].path !== "string";
  if (Array.isArray(value) && !single) {
    value.forEach(one);
  } else if (value !== undefined && value !== null) {
    one(value);
  }
  return out;
}

// presetPackages lists the packages a plugin configuration loads: the
// conventional-changelog preset, a `config` package and a releaseRules module
// (commit-analyzer lib/load-parser-config.js and lib/load-release-rules.js).
export function presetPackages(config) {
  const out = [];
  if (typeof config.preset === "string" && config.preset) out.push(`conventional-changelog-${config.preset.toLowerCase()}`);
  if (isPackage(config.config)) out.push(packageName(config.config));
  if (isPackage(config.releaseRules)) out.push(packageName(config.releaseRules));
  return out;
}

// neededPackages lists, sorted, the npm packages a semantic-release
// configuration (after its `extends` merge) loads: plugins of the plugins
// list and of per-step values, and their presets and config modules.
export function neededPackages(options) {
  const packages = new Set();
  const plugins = options.plugins ?? DEFAULT_PLUGINS;
  const refs = [...pluginRefs(plugins), ...STEPS.flatMap((step) => pluginRefs(options[step]))];
  for (const [name, cfg] of refs) {
    if (isPackage(name)) packages.add(packageName(name));
    // A plugin's config is merged over the global options (normalize.js).
    presetPackages({ ...options, ...cfg }).forEach((p) => packages.add(p));
  }
  presetPackages(options).forEach((p) => packages.add(p));
  return [...packages].sort();
}

// commitScopes reads the scopes of a conventional commit header
// ("type(scope)!: subject"; several scopes may be comma separated).
export function commitScopes(message) {
  const header = (message || "").split("\n", 1)[0];
  const match = /^[\w-]+\(([^)]*)\)!?:/.exec(header);
  if (!match) return [];
  return match[1].split(",").map((s) => s.trim()).filter(Boolean);
}

// pathMatcher returns whether a file is under one of paths: "." (everything),
// the path itself, its subtree, or a glob match (isMatch, micromatch's).
export function pathMatcher(paths, isMatch) {
  return (file) =>
    paths.some((p) => p === "." || file === p || file.startsWith(`${p}/`) || isMatch(file, p, { dot: true }));
}

// DIFF_TREE_ARGS lists the files of every commit read on stdin in one git
// call. The rule for a merge commit is --diff-merges=first-parent: its files
// are what it brings into the branch (its diff against its first parent),
// never the other side's history; the merged commits themselves are separate
// commits with their own files. (`-m --first-parent` is not that: diff-tree
// ignores --first-parent and lists the union of the diffs against every
// parent.) --diff-merges needs git 2.31 or newer
// (checked in Go before any run that has a path filter, gitversion.go).
export const DIFF_TREE_ARGS = [
  "diff-tree",
  "--stdin",
  "-z",
  "-r",
  "--root",
  "--name-only",
  "--no-renames",
  "--diff-merges=first-parent",
  "--format=%x01%H",
];

// parseDiffTree reads DIFF_TREE_ARGS output: for each commit "\x01<hash>\0",
// then "\n<first file>\0" and "<file>\0" for the others.
export function parseDiffTree(output) {
  const files = new Map();
  let current = null;
  let first = false;
  for (const token of output.split("\0")) {
    if (token.startsWith("\x01")) {
      current = token.slice(1).trim();
      files.set(current, []);
      first = true;
      continue;
    }
    const file = first && token.startsWith("\n") ? token.slice(1) : token;
    first = false;
    if (current !== null && file !== "") files.get(current).push(file);
  }
  return files;
}

// commitFileLister returns listFiles(hashes): the files of each commit, read
// with one git call (git(args, stdin) -> stdout) for the commits not in known
// yet. known is the caller's cache (hash -> files), shared by the analysis
// and notes passes and persisted across projects by the runner.
export function commitFileLister(git, known = new Map()) {
  return (hashes) => {
    const missing = [...new Set(hashes)].filter((h) => !known.has(h));
    if (missing.length > 0) {
      const listed = parseDiffTree(git(DIFF_TREE_ARGS, `${missing.join("\n")}\n`));
      for (const h of missing) known.set(h, listed.get(h) || []);
    }
    return known;
  };
}

// makeFilter keeps the commits of one project: a header scope in
// filter.scopes, or a file under filter.paths. Commits kept by scope need no
// file listing; the others are listed together, once per pass.
export function makeFilter(filter, { isMatch, listFiles }) {
  if (!filter) return (commits) => commits;
  const scopes = new Set(filter.scopes || []);
  const paths = filter.paths || [];
  const touches = pathMatcher(paths, isMatch);
  const byScope = (c) => commitScopes(c.message).some((s) => scopes.has(s));
  return (commits) => {
    const rest = commits.filter((c) => !byScope(c));
    const files = paths.length > 0 && rest.length > 0 ? listFiles(rest.map((c) => c.hash)) : new Map();
    return commits.filter((c) => byScope(c) || (files.get(c.hash) || []).some(touches));
  };
}

// raiseReleaseType applies depends_on to the analyzer's result: minimum is
// the highest bump among the dependencies released in this run (computed by
// NERV), and the project gets at least that. A null analyzer result (no
// releasable commit of its own) with a minimum gives the minimum: a released
// dependency alone releases the dependent. order is semantic-release's
// RELEASE_TYPE, lowest first.
export function raiseReleaseType(analyzed, minimum, order) {
  if (!minimum) return analyzed;
  if (analyzed === null || analyzed === undefined) return minimum;
  return order.indexOf(minimum) > order.indexOf(analyzed) ? minimum : analyzed;
}

// NO_REMOTE_URL stands in for the repository URL of the release notes when
// the repository has no public address (no repositoryUrl in its
// configuration, no package.json repository and no public origin, or a local
// path there). release-notes-generator needs a URL it can parse and its
// templates link the version header (compare) whenever there is a previous
// tag, whatever the URL; stripNoRemoteLinks then turns every link to this
// placeholder back into plain text. The .invalid top-level domain is reserved
// (RFC 2606), so the placeholder can never be a real repository.
export const NO_REMOTE_URL = "https://nerv-no-remote.invalid/repository/repository";

// notesRepositoryUrl is the URL the notes are generated with: the
// repository's own when it is a public address, NO_REMOTE_URL otherwise.
export function notesRepositoryUrl(own) {
  if (!own) return NO_REMOTE_URL;
  const local =
    own.startsWith("file:") || own.startsWith("/") || own.startsWith("\\") || /^[A-Za-z]:[\\/]/.test(own) || /^\.\.?[\\/]/.test(own);
  return local ? NO_REMOTE_URL : own;
}

const placeholderLinkRe = new RegExp(`\\[([^\\]]*)\\]\\(${NO_REMOTE_URL.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")}[^)]*\\)`, "g");

// stripNoRemoteLinks replaces every Markdown link to NO_REMOTE_URL with its
// text; other links are kept.
export function stripNoRemoteLinks(notes) {
  return typeof notes === "string" ? notes.replace(placeholderLinkRe, "$1") : notes;
}

// VERSION_MISMATCH is the code of versionGuard's error.
export const VERSION_MISMATCH = "ENERVVERSIONMISMATCH";

// versionGuard throws when semantic-release's next version (actual) is not
// the version nerv ship computed and the user confirmed (expected). The
// error carries both versions and no `semanticRelease` flag, so
// semantic-release does not call the fail plugins for it.
export function versionGuard(expected, actual) {
  if (actual === expected) return;
  const error = new Error(
    `semantic-release would release ${actual}, but nerv ship computed and confirmed ${expected}; stopped before the tag, nothing was released`,
  );
  error.code = VERSION_MISMATCH;
  error.expected = expected;
  error.actual = actual;
  throw error;
}

// failureResult is the runner's result for a failed run: every error's
// "<code> <message>", and, when one of them is versionGuard's, a structured
// versionMismatch {expected, actual} the Go side checks instead of the text.
export function failureResult(error) {
  const errors = error && Array.isArray(error.errors) ? error.errors : [error];
  const result = {
    ok: false,
    error: errors.map((e) => (e && e.code ? `${e.code} ${e.message}` : String(e && e.message ? e.message : e))).join("; "),
  };
  const mismatch = errors.find((e) => e && e.code === VERSION_MISMATCH);
  if (mismatch) result.versionMismatch = { expected: mismatch.expected, actual: mismatch.actual };
  return result;
}

const CHANGELOG_PLUGIN = "@semantic-release/changelog";
const GIT_PLUGIN = "@semantic-release/git";
// The plugins' own defaults (@semantic-release/changelog 6 and
// @semantic-release/git 10, lib/resolve-config.js).
const DEFAULT_CHANGELOG_FILE = "CHANGELOG.md";
const DEFAULT_GIT_ASSETS = ["CHANGELOG.md", "package.json", "package-lock.json", "npm-shrinkwrap.json"];

// rewriteChangelogPlugins returns the repository's plugin list with one
// project's changelog: every @semantic-release/changelog entry writes
// changelog (repository-relative), and every @semantic-release/git entry
// commits it instead of the file(s) the changelog entries pointed at (only
// those exact paths are dropped; every other asset is kept; `assets: false`
// is left alone). Other plugins are untouched, and the input is not
// modified. Without a changelog plugin nothing changes (applied false).
export function rewriteChangelogPlugins(plugins, changelog) {
  const list = Array.isArray(plugins) ? plugins : [];
  const split = (p) => (Array.isArray(p) ? [p[0], { ...(p[1] || {}) }] : [p, {}]);
  const replaced = new Set();
  let applied = false;
  const step1 = list.map((p) => {
    const [name, options] = split(p);
    if (name !== CHANGELOG_PLUGIN) return p;
    applied = true;
    replaced.add(options.changelogFile == null ? DEFAULT_CHANGELOG_FILE : options.changelogFile);
    return [name, { ...options, changelogFile: changelog }];
  });
  if (!applied) return { plugins: list, applied };
  const assetPath = (a) => (a && typeof a === "object" ? a.path : a);
  const out = step1.map((p) => {
    const [name, options] = split(p);
    if (name !== GIT_PLUGIN || options.assets === false) return p;
    const assets = options.assets == null ? DEFAULT_GIT_ASSETS : [].concat(options.assets);
    const kept = assets.filter((a) => assetPath(a) === changelog || !replaced.has(assetPath(a)));
    if (!kept.some((a) => assetPath(a) === changelog)) kept.push(changelog);
    return [name, { ...options, assets: kept }];
  });
  return { plugins: out, applied };
}

// DIM_NOTES_OPTIONS are dim.comprobantes.api's commit-analyzer and
// release-notes-generator options (its package.json "release").
const DIM_NOTES_OPTIONS = () => ({
  preset: "conventionalcommits",
  parserOpts: { noteKeywords: ["BREAKING CHANGE", "BREAKING CHANGES", "BREAKING"] },
});

// defaultConfig is NERV's built-in semantic-release configuration, used when
// a repository has none of its own: dim.comprobantes.api's package.json
// "release" without its repository-specific @semantic-release/npm
// (npmPublish false) and @semantic-release/exec (csproj bump, CI outputs)
// entries, which nerv's presets and outputs replace; the changelog at the
// root (a project's changelog is applied afterwards by
// rewriteChangelogPlugins), committed by @semantic-release/git with its
// default message (dim sets none). branches are {name, prerelease?,
// channel?} (the pipeline: section's, or its defaults, sent by Go); a branch
// with neither is written as its bare name. @semantic-release/github is
// included only when github is true (origin on GitHub): elsewhere it fails
// its checks (EINVALIDGITHUBURL). Fresh objects on every call.
export function defaultConfig(branches, { github = true } = {}) {
  if (!Array.isArray(branches) || branches.length === 0) {
    throw new Error("the built-in semantic-release configuration needs its branches");
  }
  return {
    branches: branches.map(({ name, prerelease, channel }) =>
      prerelease || channel ? { name, ...(prerelease ? { prerelease } : {}), ...(channel ? { channel } : {}) } : name,
    ),
    plugins: [
      ["@semantic-release/commit-analyzer", DIM_NOTES_OPTIONS()],
      ["@semantic-release/release-notes-generator", DIM_NOTES_OPTIONS()],
      ["@semantic-release/changelog", { changelogFile: "CHANGELOG.md" }],
      ["@semantic-release/git", { assets: ["CHANGELOG.md"] }],
      ...(github ? ["@semantic-release/github"] : []),
    ],
  };
}
