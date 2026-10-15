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
//    "filter": {"paths": [...], "scopes": [...]} | null, "minReleaseType",
//    "commitFilesCache"}
//     One semantic-release dry run in cwd (a worktree on the requested branch).
//     commitFilesCache is a JSON file (commit hash -> files) shared by the
//     projects of one NERV run.
//   {"mode":"plan"} also returns configFile: the repository's configuration
//   file (cwd-relative) or "" when it has none; run and release then get
//   "useDefault": true and "branches" and use defaultConfig(branches).
//   Both run and release take an optional "changelog": the project's
//   changelog file, applied to the repository's @semantic-release/changelog
//   and @semantic-release/git entries (rewriteChangelogPlugins); the result
//   says changelogIgnored when the configuration has no changelog plugin.
//   {"mode":"release", ... as run ..., "expectedVersion"}
//     One REAL semantic-release run (nerv ship) in cwd, a clone of the user's
//     repository checked out on origin's commit of the branch, pushing to
//     repositoryUrl (origin's address without credentials; git rewrites it
//     to the real URL through its environment). Every step is the
//     repository's own (verifyConditions, prepare, publish, success, fail,
//     ...), except that analyzeCommits and generateNotes are wrapped as in
//     run mode (project commit filter, depends_on raise, notes links) and
//     verifyRelease first checks that the next version is expectedVersion:
//     any other version throws ENERVVERSIONMISMATCH there (reported as the
//     result's structured versionMismatch field), before prepare and
//     the tag (lib/definitions/plugins.js order: verifyConditions,
//     analyzeCommits, verifyRelease, generateNotes, prepare, then the tag and
//     push in index.js, then publish, success). The error has no
//     `semanticRelease` flag, so semantic-release does not call the fail
//     plugins for it (index.js callFail keeps only errors with that flag).
//
// The pure logic (package listing, commit filter, depends_on raise, result
// writing) lives in nerv-semrel-lib.mjs, embedded next to this file.
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
//   what semantic-release would use; the mirror's origin is set to the
//   user's public origin) so their links match a CI run; with no public URL
//   the notes have no links.

import { createRequire } from "node:module";
import { execFileSync } from "node:child_process";
import { readFileSync, writeFileSync } from "node:fs";
import path from "node:path";
import { pathToFileURL } from "node:url";

import {
  NO_REMOTE_URL,
  commitFileLister,
  defaultConfig,
  failureResult,
  finish,
  isPackage,
  makeFilter,
  neededPackages,
  notesRepositoryUrl,
  packageName,
  raiseReleaseType,
  rewriteChangelogPlugins,
  stripNoRemoteLinks,
  versionGuard,
} from "./nerv-semrel-lib.mjs";

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

async function plan({ cacheDir, cwd }) {
  const m = cacheModules(cacheDir);
  const cosmi = await m.load("cosmiconfig");
  const cosmiconfig = cosmi.cosmiconfig || cosmi.default.cosmiconfig;
  const importFrom = (await m.load("import-from-esm")).default;
  // semantic-release's own search (lib/get-config.js: cosmiconfig("release")
  // in cwd only); nothing found means the built-in default is used.
  const found = await cosmiconfig("release").search(cwd);
  if (!found) {
    return { ok: true, packages: neededPackages(defaultConfig([{ name: "main" }])).sort(), unresolved: [], configFile: "" };
  }
  const { config = {} } = found;

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

  neededPackages(options).forEach((p) => packages.add(p));
  return { ok: true, packages: [...packages].sort(), unresolved, configFile: path.relative(cwd, found.filepath) || found.filepath };
}

// loadKnownFiles reads the per-run cache of commit files (hash -> files)
// shared by the projects of one NERV run; a missing or unreadable file starts
// empty.
function loadKnownFiles(file) {
  if (!file) return new Map();
  try {
    return new Map(Object.entries(JSON.parse(readFileSync(file, "utf8"))));
  } catch {
    return new Map();
  }
}

function saveKnownFiles(file, known) {
  if (file) writeFileSync(file, JSON.stringify(Object.fromEntries(known)));
}

async function run(req) {
  const { cacheDir, cwd, repositoryUrl, tagFormat, filter, minReleaseType, commitFilesCache, expectedVersion, changelog, useDefault, branches } = req;
  const real = req.mode === "release";
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
  // A real run pushes to repositoryUrl and its notes link to it (never to
  // the clone's remote.origin.url, which git may rewrite to a URL holding
  // credentials).
  if (real) baseOptions.repositoryUrl = repositoryUrl;
  // No configuration of the repository's own: the built-in one (dim's), as
  // API options (merged over the empty repository configuration).
  if (useDefault) Object.assign(baseOptions, defaultConfig(branches));
  // The repository's own steps, built exactly as semantic-release builds them.
  const context = { cwd, env, stdout: process.stderr, stderr: process.stderr };
  context.logger = getLogger(context);
  let { plugins: own, options: ownOptions } = await getConfig(context, baseOptions);
  // A project's own changelog (monorepo, or set explicitly): the
  // repository's plugin list with its changelog and git entries pointed at
  // it, passed as the `plugins` API option (which replaces the config's)
  // and loaded again so the wrapped steps use the same configuration.
  let changelogIgnored = false;
  if (changelog) {
    const rewritten = rewriteChangelogPlugins(ownOptions.plugins, changelog);
    if (rewritten.applied) {
      baseOptions.plugins = rewritten.plugins;
      ({ plugins: own, options: ownOptions } = await getConfig(context, baseOptions));
    } else {
      changelogIgnored = true;
    }
  }
  // The notes link to the repository's public address: its configured
  // repositoryUrl, package.json repository or origin (the mirror's origin is
  // the user's public origin, or absent; see internal/semrel withCheckout).
  // Without one (or with a local path) they are generated against a
  // placeholder whose links are then removed, instead of links such as
  // https:/home/me/repo/compare/....
  const publicUrl = notesRepositoryUrl(ownOptions.repositoryUrl);
  // The files of the commits, listed with one git call per pass for the
  // commits not listed yet in this NERV run (see commitFileLister).
  const knownFiles = loadKnownFiles(commitFilesCache);
  const git = (args, input) => execFileSync("git", args, { cwd, input, encoding: "utf8", maxBuffer: 1 << 30 });
  const select = makeFilter(filter, { isMatch: (file, glob, options) => micromatch.isMatch(file, glob, options), listFiles: commitFileLister(git, knownFiles) });

  let lastRelease = null;
  async function nervAnalyzeCommits(_pluginConfig, input) {
    lastRelease = input.lastRelease;
    const analyzed = await own.analyzeCommits({ ...input, commits: select(input.commits) });
    return raiseReleaseType(analyzed, minReleaseType, RELEASE_TYPE);
  }
  async function nervGenerateNotes(_pluginConfig, input) {
    const notes = await own.generateNotes({
      ...input,
      commits: select(input.commits),
      options: { ...input.options, repositoryUrl: publicUrl },
    });
    return publicUrl === NO_REMOTE_URL ? stripNoRemoteLinks(notes) : notes;
  }

  async function nervVerifyRelease(_pluginConfig, input) {
    versionGuard(expectedVersion, input.nextRelease && input.nextRelease.version);
    return own.verifyRelease(input);
  }

  const steps = real
    ? {
        dryRun: false,
        analyzeCommits: [[nervAnalyzeCommits, {}]],
        verifyRelease: [[nervVerifyRelease, {}]],
        generateNotes: [[nervGenerateNotes, {}]],
      }
    : {
        dryRun: true,
        verifyConditions: [],
        analyzeCommits: [[nervAnalyzeCommits, {}]],
        verifyRelease: [],
        generateNotes: [[nervGenerateNotes, {}]],
        prepare: [],
        publish: [],
        addChannel: [],
        success: [],
        fail: [],
      };
  const result = await semanticRelease(
    { ...baseOptions, ci: false, repositoryUrl, ...steps },
    { cwd, env, stdout: process.stderr, stderr: process.stderr },
  );
  saveKnownFiles(commitFilesCache, knownFiles);

  const last = (r) => (r && r.version ? { version: r.version, gitHead: r.gitHead || "", gitTag: r.gitTag || "" } : null);
  if (!result || !result.nextRelease) {
    return { ok: true, released: false, lastRelease: last(lastRelease), changelogIgnored };
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
    changelogIgnored,
  };
}

try {
  const req = await readStdin();
  // run and release modes share run(), which reads req.mode.
  const result = req.mode === "plan" ? await plan(req) : await run(req);
  finish(process.stdout, result, 0);
} catch (error) {
  finish(process.stdout, failureResult(error), 1);
}
