// nerv-semrel.mjs: NERV's node runner around semantic-release's programmatic
// API. Embedded in the nerv binary (internal/semrel) and run as
// `node nerv-semrel.mjs` with one JSON request on stdin. It writes exactly one
// result line, prefixed by RESULT_MARKER, to stdout; semantic-release's own
// logs go to stderr.
//
// Requests:
//   {"mode":"plan", "cacheDir", "cwd"}
//     Loads the repository's semantic-release configuration the way
//     semantic-release does and lists the npm packages it needs (plugins,
//     shareable configs, conventional-changelog presets), without loading them.
//   {"mode":"run", "cacheDir", "cwd", "repositoryUrl", "tagFormat",
//    "filter": {"paths": [...], "scopes": [...]} | null, "minReleaseType"}
//     One semantic-release dry run in cwd (a worktree on the requested branch).
//
// What this relies on (semantic-release 24.2.x source, lib/ and index.js):
// - index.js default export `(cliOptions, {cwd, env, stdout, stderr})` is the
//   programmatic API; lib/get-config.js loads the repository configuration
//   with cosmiconfig("release") (.releaserc*, release.config.*, package.json
//   "release"), merges `extends` shareable configs, then merges the API
//   options over it: `options = {...config, ...cliOptions}`. So `tagFormat`
//   passed here overrides the repository's tagFormat per project, and
//   `ci: false` sets `noCi`.
// - lib/plugins/index.js builds each step from `options.plugins`, then
//   `options = {...plugins, ...options}`: a step key given in the API options
//   (e.g. `verifyConditions: []`) replaces the plugin-provided steps of that
//   type. lib/plugins/utils.js validateStep accepts an empty list for every
//   step except analyzeCommits, and `[fn, config]` pairs (inline functions)
//   for any step; lib/plugins/normalize.js calls them as
//   `fn(pluginConfig, context)`.
// - lib/definitions/plugins.js: in a dry run verifyConditions,
//   analyzeCommits, verifyRelease and generateNotes still run; prepare,
//   publish, addChannel, success and fail are skipped. verifyConditions is
//   where @semantic-release/github needs GH_TOKEN and @semantic-release/npm
//   needs NPM_TOKEN (unless npmPublish is false); verifyRelease is where
//   @semantic-release/exec runs verifyReleaseCmd. This read-only run empties
//   verifyConditions, verifyRelease (and, for safety, every side-effect step)
//   and wraps analyzeCommits and generateNotes around the repository's own
//   configured steps, so the version and the notes are computed exactly as
//   the repository's configuration computes them.
// - index.js run(): even in a dry run semantic-release lists the branches
//   with `git ls-remote --heads <repositoryUrl>`, fetches every release
//   branch with `git fetch --tags <repositoryUrl>
//   +refs/heads/<b>:refs/heads/<b>` and checks push permission with
//   `git push --dry-run <repositoryUrl> HEAD:<branch>` (lib/git.js,
//   lib/branches/index.js, lib/get-git-auth-url.js). NERV runs in a worktree
//   of a disposable mirror of the repository (which already holds origin's
//   release tags and notes, see internal/semrel withCheckout) and passes that
//   mirror as repositoryUrl: branches are the local ones, every fetch is a
//   no-op on the same refs and the push check needs no credentials. The
//   release notes still get the repository's public URL (the configured
//   repositoryUrl, package.json repository or remote.origin.url, exactly
//   what semantic-release would use) so their
//   links match a CI run.

import { createRequire } from "node:module";
import { execFileSync } from "node:child_process";
import path from "node:path";
import { pathToFileURL } from "node:url";

const RESULT_MARKER = "NERV_SEMREL_RESULT ";

function emit(result) {
  process.stdout.write(`\n${RESULT_MARKER}${JSON.stringify(result)}\n`);
}

async function readStdin() {
  const chunks = [];
  for await (const chunk of process.stdin) chunks.push(chunk);
  return JSON.parse(Buffer.concat(chunks).toString("utf8"));
}

// cacheModules resolves semantic-release and its own dependencies from the
// NERV cache, never from the repository.
function cacheModules(cacheDir) {
  const srPkg = path.join(cacheDir, "node_modules", "semantic-release", "package.json");
  const srDir = path.dirname(srPkg);
  const req = createRequire(srPkg);
  const load = (name) => import(pathToFileURL(req.resolve(name)).href);
  const lib = (file) => import(pathToFileURL(path.join(srDir, "lib", file)).href);
  return { srDir, req, load, lib, index: () => import(pathToFileURL(path.join(srDir, "index.js")).href) };
}

const STEPS = [
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

// isPackage tells a bare npm specifier from a file path.
function isPackage(name) {
  return typeof name === "string" && name !== "" && !/^(\.|\/|[A-Za-z]:[\\/]|\\\\|file:)/.test(name);
}

// packageName drops a subpath: "@scope/pkg/x" -> "@scope/pkg", "pkg/x" -> "pkg".
function packageName(spec) {
  const parts = spec.split("/");
  return spec.startsWith("@") ? parts.slice(0, 2).join("/") : parts[0];
}

// pluginRefs lists [name, config] for every plugin reference in a plugins
// list or a step value (string, [name, config], {path, ...config}).
function pluginRefs(value) {
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
function presetPackages(config) {
  const out = [];
  if (typeof config.preset === "string" && config.preset) out.push(`conventional-changelog-${config.preset.toLowerCase()}`);
  if (isPackage(config.config)) out.push(packageName(config.config));
  if (isPackage(config.releaseRules)) out.push(packageName(config.releaseRules));
  return out;
}

async function plan({ cacheDir, cwd }) {
  const m = cacheModules(cacheDir);
  const cosmi = await m.load("cosmiconfig");
  const cosmiconfig = cosmi.cosmiconfig || cosmi.default.cosmiconfig;
  const importFrom = (await m.load("import-from-esm")).default;
  const { config = {} } = (await cosmiconfig("release").search(cwd)) || {};

  const packages = new Set();
  const unresolved = [];
  let { extends: extendPaths, ...options } = config;
  if (extendPaths) {
    let merged = {};
    for (const ext of [].concat(extendPaths)) {
      if (isPackage(ext)) packages.add(packageName(ext));
      let loaded;
      try {
        loaded = (await importFrom.silent(path.join(m.srDir, "lib"), ext)) || (await importFrom(cwd, ext));
      } catch {
        unresolved.push(ext);
        continue;
      }
      merged = { ...merged, ...(loaded.default && !loaded.plugins ? loaded.default : loaded) };
    }
    options = { ...merged, ...options };
  }

  const plugins = options.plugins ?? [
    "@semantic-release/commit-analyzer",
    "@semantic-release/release-notes-generator",
    "@semantic-release/npm",
    "@semantic-release/github",
  ];
  const refs = [...pluginRefs(plugins), ...STEPS.flatMap((step) => pluginRefs(options[step]))];
  for (const [name, cfg] of refs) {
    if (isPackage(name)) packages.add(packageName(name));
    // A plugin's config is merged over the global options (normalize.js).
    presetPackages({ ...options, ...cfg }).forEach((p) => packages.add(p));
  }
  presetPackages(options).forEach((p) => packages.add(p));
  return { ok: true, packages: [...packages].sort(), unresolved };
}

// commitScopes reads the scopes of a conventional commit header
// ("type(scope)!: subject"; several scopes may be comma separated).
function commitScopes(message) {
  const header = (message || "").split("\n", 1)[0];
  const match = /^[\w-]+\(([^)]*)\)!?:/.exec(header);
  if (!match) return [];
  return match[1].split(",").map((s) => s.trim()).filter(Boolean);
}

function makeFilter(filter, cwd, micromatch) {
  if (!filter) return (commits) => commits;
  const paths = filter.paths || [];
  const scopes = new Set(filter.scopes || []);
  const touches = (file) =>
    paths.some((p) => p === "." || file === p || file.startsWith(`${p}/`) || micromatch.isMatch(file, p, { dot: true }));
  const files = (hash) =>
    execFileSync("git", ["diff-tree", "--no-commit-id", "--name-only", "-r", "--root", "-m", "--first-parent", hash], {
      cwd,
      encoding: "utf8",
    })
      .split("\n")
      .filter(Boolean);
  return (commits) =>
    commits.filter((c) => commitScopes(c.message).some((s) => scopes.has(s)) || files(c.hash).some(touches));
}

async function run(req) {
  const { cacheDir, cwd, repositoryUrl, tagFormat, filter, minReleaseType } = req;
  const m = cacheModules(cacheDir);
  const semanticRelease = (await m.index()).default;
  const getConfig = (await m.lib("get-config.js")).default;
  const getLogger = (await m.lib("get-logger.js")).default;
  const { RELEASE_TYPE } = await m.lib("definitions/constants.js");
  const envCi = (await m.load("env-ci")).default;
  const micromatch = (await m.load("micromatch")).default;

  const env = { ...process.env };
  const ci = envCi({ env, cwd });
  if (ci.isCi && ci.service && ci.service !== "git") {
    throw new Error(`a CI environment (${ci.name}) was detected in the environment; NERV computes versions from the local branch only`);
  }

  const baseOptions = tagFormat ? { tagFormat } : {};
  // The repository's own steps, built exactly as semantic-release builds them.
  const context = { cwd, env, stdout: process.stderr, stderr: process.stderr };
  context.logger = getLogger(context);
  const { plugins: own, options: ownOptions } = await getConfig(context, baseOptions);
  // release-notes-generator parses the URL with `new URL()`; a local path
  // (a remote on disk, or no remote at all) becomes a file: URL instead of
  // failing.
  const ownUrl = ownOptions.repositoryUrl;
  const publicUrl = !ownUrl
    ? pathToFileURL(repositoryUrl).href
    : path.isAbsolute(ownUrl)
      ? pathToFileURL(ownUrl).href
      : ownUrl;
  const select = makeFilter(filter, cwd, micromatch);

  let lastRelease = null;
  const raise = (type) => {
    if (!minReleaseType) return type;
    return RELEASE_TYPE.indexOf(minReleaseType) > RELEASE_TYPE.indexOf(type) ? minReleaseType : type;
  };
  async function nervAnalyzeCommits(_pluginConfig, input) {
    lastRelease = input.lastRelease;
    return raise(await own.analyzeCommits({ ...input, commits: select(input.commits) }));
  }
  async function nervGenerateNotes(_pluginConfig, input) {
    return own.generateNotes({
      ...input,
      commits: select(input.commits),
      options: { ...input.options, repositoryUrl: publicUrl },
    });
  }

  const result = await semanticRelease(
    {
      ...baseOptions,
      dryRun: true,
      ci: false,
      repositoryUrl,
      verifyConditions: [],
      analyzeCommits: [[nervAnalyzeCommits, {}]],
      verifyRelease: [],
      generateNotes: [[nervGenerateNotes, {}]],
      prepare: [],
      publish: [],
      addChannel: [],
      success: [],
      fail: [],
    },
    { cwd, env, stdout: process.stderr, stderr: process.stderr },
  );

  const last = (r) => (r && r.version ? { version: r.version, gitHead: r.gitHead || "", gitTag: r.gitTag || "" } : null);
  if (!result || !result.nextRelease) {
    return { ok: true, released: false, lastRelease: last(lastRelease) };
  }
  const n = result.nextRelease;
  return {
    ok: true,
    released: true,
    nextRelease: {
      type: n.type,
      version: n.version,
      channel: n.channel || "",
      gitHead: n.gitHead || "",
      gitTag: n.gitTag || "",
      notes: n.notes || "",
    },
    lastRelease: last(result.lastRelease),
  };
}

try {
  const req = await readStdin();
  const result = req.mode === "plan" ? await plan(req) : await run(req);
  emit(result);
  process.exit(0);
} catch (error) {
  const errors = error && Array.isArray(error.errors) ? error.errors : [error];
  emit({
    ok: false,
    error: errors.map((e) => (e && e.code ? `${e.code} ${e.message}` : String(e && e.message ? e.message : e))).join("; "),
  });
  process.exit(1);
}
