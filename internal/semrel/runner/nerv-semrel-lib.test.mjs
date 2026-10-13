// Unit tests of the runner's pure logic. Run with
//   node --test internal/semrel/runner/nerv-semrel-lib.test.mjs
// (also run by `go test ./internal/semrel/` when node is in PATH).

import { test } from "node:test";
import assert from "node:assert/strict";
import { execFileSync, spawn } from "node:child_process";
import { mkdtempSync, rmSync, writeFileSync, mkdirSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

import {
  RESULT_MARKER,
  commitFileLister,
  commitScopes,
  makeFilter,
  neededPackages,
  parseDiffTree,
  pathMatcher,
  raiseReleaseType,
} from "./nerv-semrel-lib.mjs";

const libURL = pathToFileURL(path.join(path.dirname(fileURLToPath(import.meta.url)), "nerv-semrel-lib.mjs")).href;

// globMatch is a small stand-in for micromatch.isMatch: ** spans
// directories, * stays within one.
function globMatch(file, glob) {
  const re = glob
    .split("**")
    .map((part) => part.split("*").map((s) => s.replace(/[.+?^${}()|[\]\\]/g, "\\$&")).join("[^/]*"))
    .join(".*");
  return new RegExp(`^${re}$`).test(file);
}

const RELEASE_TYPE = ["patch", "minor", "major"];

test("commitScopes reads comma-separated scopes and the breaking marker", () => {
  assert.deepEqual(commitScopes("feat(api): add x"), ["api"]);
  assert.deepEqual(commitScopes("feat(api,web): add x"), ["api", "web"]);
  assert.deepEqual(commitScopes("fix(api, web )!: drop y\n\nBREAKING CHANGE: z"), ["api", "web"]);
  assert.deepEqual(commitScopes("feat(api)!: breaking"), ["api"]);
  assert.deepEqual(commitScopes("fix!: no scope"), []);
  assert.deepEqual(commitScopes("feat: no scope"), []);
  assert.deepEqual(commitScopes("Merge branch 'feature/x' into develop"), []);
  assert.deepEqual(commitScopes(""), []);
  assert.deepEqual(commitScopes(undefined), []);
  // Only the header counts.
  assert.deepEqual(commitScopes("chore: x\n\nfeat(api): not a header"), []);
});

test("pathMatcher matches a path, its subtree and globs", () => {
  const touches = pathMatcher(["api", "libs/**", "shared/*.proto"], globMatch);
  assert.equal(touches("api"), true);
  assert.equal(touches("api/src/a.cs"), true);
  assert.equal(touches("apiary/a.cs"), false, "a sibling with the same prefix is not inside the path");
  assert.equal(touches("libs/core/x.ts"), true);
  assert.equal(touches("shared/a.proto"), true);
  assert.equal(touches("shared/deep/a.proto"), false);
  assert.equal(touches("web/a.ts"), false);
  assert.equal(pathMatcher(["."], globMatch)("anything/at/all"), true);
});

test("parseDiffTree reads the -z name-only output, merges and empty commits", () => {
  const out =
    "\x01aaa\x00\nc d/ñ é.txt\x00two.txt\x00" + // first file follows the header's newline
    "\x01bbb\x00\nb/y\x00" +
    "\x01ccc\x00"; // a commit without changes
  const files = parseDiffTree(out);
  assert.deepEqual(files.get("aaa"), ["c d/ñ é.txt", "two.txt"]);
  assert.deepEqual(files.get("bbb"), ["b/y"]);
  assert.deepEqual(files.get("ccc"), []);
});

test("commitFileLister runs git once per new set of commits and caches them", () => {
  const calls = [];
  const git = (args, input) => {
    calls.push({ args, input });
    return input
      .trim()
      .split("\n")
      .map((h) => `\x01${h}\x00\n${h}/file\x00`)
      .join("");
  };
  const known = new Map([["old", ["cached/file"]]]);
  const list = commitFileLister(git, known);
  let files = list(["h1", "h2", "old", "h1"]);
  assert.equal(calls.length, 1);
  assert.equal(calls[0].input, "h1\nh2\n");
  assert.ok(calls[0].args.includes("--stdin") && calls[0].args.includes("--diff-merges=first-parent"));
  assert.deepEqual(files.get("h1"), ["h1/file"]);
  assert.deepEqual(files.get("old"), ["cached/file"]);
  files = list(["h1", "h2"]); // a second pass (generateNotes): no git
  assert.equal(calls.length, 1);
  list(["h3"]);
  assert.equal(calls.length, 2);
  assert.equal(known.size, 4, "the caller's map holds every commit, to persist across projects");
  // A commit git prints nothing for (no changes) is cached as empty.
  const silent = commitFileLister(() => "", new Map());
  assert.deepEqual(silent(["e"]).get("e"), []);
});

test("commitFileLister on a real repository: merges count their first-parent changes", { skip: !hasGit() }, () => {
  const dir = mkdtempSync(path.join(tmpdir(), "nerv-semrel-lib-"));
  try {
    const git = (...args) =>
      execFileSync("git", ["-C", dir, ...args], {
        encoding: "utf8",
        env: { ...process.env, GIT_CONFIG_GLOBAL: "/dev/null", GIT_CONFIG_NOSYSTEM: "1", GIT_AUTHOR_NAME: "t", GIT_AUTHOR_EMAIL: "t@e", GIT_COMMITTER_NAME: "t", GIT_COMMITTER_EMAIL: "t@e" },
      }).trim();
    const write = (rel) => {
      mkdirSync(path.dirname(path.join(dir, rel)), { recursive: true });
      writeFileSync(path.join(dir, rel), rel);
    };
    git("init", "-q", "-b", "main");
    write("root.txt");
    git("add", "-A");
    git("commit", "-qm", "root");
    git("checkout", "-qb", "feature");
    write("web/a.ts");
    git("add", "-A");
    git("commit", "-qm", "feat(web): a");
    const feature = git("rev-parse", "HEAD");
    git("checkout", "-q", "main");
    write("api/b.cs");
    git("add", "-A");
    git("commit", "-qm", "fix(api): b");
    const onMain = git("rev-parse", "HEAD");
    git("merge", "-q", "--no-ff", "feature", "-m", "Merge branch 'feature'");
    const merge = git("rev-parse", "HEAD");
    const root = git("rev-list", "--max-parents=0", "HEAD");

    const run = (args, input) => execFileSync("git", ["-C", dir, ...args], { input, encoding: "utf8", maxBuffer: 1 << 28 });
    const files = commitFileLister(run)([merge, onMain, feature, root]);
    assert.deepEqual(files.get(merge), ["web/a.ts"], "a merge brings what it adds to its first parent, not the other side's history");
    assert.deepEqual(files.get(onMain), ["api/b.cs"]);
    assert.deepEqual(files.get(feature), ["web/a.ts"]);
    assert.deepEqual(files.get(root), ["root.txt"]);
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
});

test("makeFilter keeps commits by scope or by path, listing files once per pass", () => {
  const files = {
    a: ["api/x.cs"],
    b: ["web/y.ts"],
    c: ["libs/core/z.ts"],
    d: ["docs/readme.md"],
    m: ["api/merged.cs"],
  };
  const listed = [];
  const listFiles = (hashes) => {
    listed.push(hashes);
    return new Map(hashes.map((h) => [h, files[h] || []]));
  };
  const select = makeFilter({ paths: ["api", "libs/**"], scopes: ["api", "core"] }, { isMatch: globMatch, listFiles });
  const commits = [
    { hash: "a", message: "chore: touch api" },
    { hash: "b", message: "feat(web,core): shared change" },
    { hash: "c", message: "fix: lib" },
    { hash: "d", message: "docs: readme" },
    { hash: "m", message: "Merge branch 'feature/x'" },
    { hash: "s", message: "feat(api)!: breaking, no files" },
  ];
  assert.deepEqual(
    select(commits).map((c) => c.hash),
    ["a", "b", "c", "m", "s"],
  );
  assert.equal(listed.length, 1, "one file listing per pass");
  assert.deepEqual(listed[0], ["a", "c", "d", "m"], "commits kept by scope need no file listing");

  assert.equal(makeFilter(null, { isMatch: globMatch, listFiles })(commits), commits);
  const scopesOnly = makeFilter({ paths: [], scopes: ["web"] }, { isMatch: globMatch, listFiles });
  assert.deepEqual(scopesOnly(commits).map((c) => c.hash), ["b"]);
  assert.equal(listed.length, 1, "no paths: no listing");
});

test("raiseReleaseType applies depends_on", () => {
  assert.equal(raiseReleaseType("minor", "", RELEASE_TYPE), "minor");
  assert.equal(raiseReleaseType(null, "", RELEASE_TYPE), null);
  assert.equal(raiseReleaseType(null, "patch", RELEASE_TYPE), "patch", "a released dependency alone releases the dependent");
  assert.equal(raiseReleaseType(undefined, "minor", RELEASE_TYPE), "minor");
  assert.equal(raiseReleaseType("patch", "minor", RELEASE_TYPE), "minor");
  assert.equal(raiseReleaseType("major", "patch", RELEASE_TYPE), "major");
  assert.equal(raiseReleaseType("minor", "minor", RELEASE_TYPE), "minor");
});

test("neededPackages lists plugins, presets and config modules", () => {
  assert.deepEqual(neededPackages({}), [
    "@semantic-release/commit-analyzer",
    "@semantic-release/github",
    "@semantic-release/npm",
    "@semantic-release/release-notes-generator",
  ]);
  assert.deepEqual(
    neededPackages({
      preset: "angular",
      plugins: [
        ["@semantic-release/commit-analyzer", { preset: "conventionalcommits", releaseRules: "@acme/rules/strict" }],
        "./local-plugin.js",
        { path: "@semantic-release/exec", verifyReleaseCmd: "x" },
        ["@semantic-release/release-notes-generator", { config: "@acme/changelog-config" }],
      ],
      generateNotes: ["semantic-release-slack-bot", { preset: "eslint" }],
      analyzeCommits: [{ path: "@semantic-release/commit-analyzer", releaseRules: "./rules.js" }],
    }),
    [
      "@acme/changelog-config",
      "@acme/rules",
      "@semantic-release/commit-analyzer",
      "@semantic-release/exec",
      "@semantic-release/release-notes-generator",
      "conventional-changelog-angular",
      "conventional-changelog-conventionalcommits",
      "conventional-changelog-eslint",
      "semantic-release-slack-bot",
    ],
  );
  // A [name, config] step value is one reference; a list of names is several.
  assert.deepEqual(neededPackages({ plugins: [], verifyConditions: ["a-plugin", "b-plugin"] }), ["a-plugin", "b-plugin"]);
  assert.deepEqual(neededPackages({ plugins: [], publish: "@scope/pub/sub" }), ["@scope/pub"]);
});

test("finish writes a large result completely before the process exits", async () => {
  const notes = "x".repeat(8 << 20);
  const script = `import { finish } from ${JSON.stringify(libURL)};
finish(process.stdout, { ok: true, notes: "x".repeat(8 << 20) }, 3);`;
  const child = spawn(process.execPath, ["--input-type=module", "-e", script], { stdio: ["ignore", "pipe", "inherit"] });
  const chunks = [];
  child.stdout.on("data", (c) => chunks.push(c));
  const code = await new Promise((resolve) => child.on("close", resolve));
  const out = Buffer.concat(chunks).toString("utf8");
  assert.equal(code, 3);
  const line = out.split("\n").find((l) => l.startsWith(RESULT_MARKER));
  assert.ok(line, `no result line in ${out.length} bytes`);
  assert.equal(JSON.parse(line.slice(RESULT_MARKER.length)).notes.length, notes.length);
});

function hasGit() {
  try {
    execFileSync("git", ["--version"]);
    return true;
  } catch {
    return false;
  }
}
