# nerv deploy publishes the release without deploying to servers

Objective: a core `publish` step that pushes the artifact's images, the git host release right after it, and server targets optional, per `~/projects/docs/nerv-deploy-registry-and-servers-plan-2026-10-26.md` (option 1).
Branch: `feat/deploy-core-publish` from `develop`, one PR. Delivery: ask-on-risk; forecast ~450 authored lines, one coherent behavior. Test runner: `go test ./...`.

## Specs

- **S1** Goal, quoted from the plan: "`nerv deploy` must do the release and nothing else: 1. Push the release commit, the tag and the channel note. 2. Publish the artifacts the release generated, when there are any (for example, the images declared in `release.projects[].images`, pushed to their registry). 3. Create the git host release (GitHub)." Rolling out to servers "must not be part of `nerv deploy`".
- **S2** Core `publish` step: "**Steps:** `verify-artifact`, `clone`, `commit`, `tag`, `notes`, `push`, **`publish`**, `deploy:<target>`/`verify:<target>` (only when targets exist), `release`." "for each `outputs/*.tar` of the manifest, run `docker load -i <tar>`. Then, for each entry of the manifest's `images` (version, channel and `latest` tags, as recorded by `nerv release`), run `docker push <image>`."
  - "Nothing to publish: the artifact has no `images` and no other publishable outputs, so the step is `skipped`."
  - "Reruns: re-pushing an image tag that is already in the registry with the same digest is a no-op. Reruns stay safe."
- **S3** Preflight: "when the artifact has images, require `docker` and check that each image's registry has credentials in the docker config. Fail before anything is pushed, with an actionable message such as `run docker login ghcr.io`."
- **S4** Configuration: "none required. An optional switch keeps today's behavior for anyone who publishes from a target": `deploy.publish` ("default true: push the artifact's images in the core publish step"); `targets: [] # optional; server rollout may live outside nerv`. "When `publish: true` is set together with a target that also publishes, the core step pushes once. The target's `publish` should then be skipped, or documented as redundant." Decision (assumption): the target's `publish` keeps running and is documented as redundant (a re-push of the same digest is a no-op), because a target's publish may publish other things (`deploy-bash` `publish_script`).
- **S5** Ordering: "Create the git host release right after `push` and `publish`, before or independently of any target's `deploy` and `verify`. The tag and the artifacts are public at that point. A failing server target must not leave the release missing." Step order: `push`, `publish`, `release`, then `publish:<target>`, `deploy:<target>`, `verify:<target>`.
- **S6** Out of scope: "NuGet and npm packages in `outputs/` would follow the same pattern later"; option 2 (`deploy-registry`) is not built.
- **S7** Tests, from the plan: artifact with images and no targets (`push`, `publish` done, `release` done; one `docker load` per tar, one `docker push` per manifest image); artifact without images (`publish` skipped, `release` done); dry run (`publish` is `dry-run`, no docker); preflight (missing docker refused with images, passes without; registry without credentials refused before `push`); rerun after a failure past `publish` resumes; `publish: false` skips the core step and target publishing still works; a failing `deploy:<target>` still leaves `release` done.
- **S8** Acceptance: with `deploy: { host: github }`, `/deploy:run` on a `main` artifact pushes commit, tag and note, pushes `ghcr.io/<org>/<image>:<version>` and `:latest`, creates the GitHub release, and connects to no server.

## Tasks

| ID | Specs | Route | Status | Commit |
|---|---|---|---|---|
| T1 | S1-S8 | inline (one behavior in internal/deployrun + deployconfig, docs with it) | implemented; RED observed (missing publish step, Publish field), GREEN `go test ./...`, `go vet ./...`; independent verifier running | 73a6112 |

Route evidence: one behavior across `internal/deployrun/run.go` and `internal/deployconfig`, no parallel unit; no writer trigger. Risk: items 3 (the report's steps and their order, consumed by `/deploy:run` and the deployer agent) and 5 (deploy): independent verifier at the end.

## Log

- **L1** (2026-10-26, user): "si, pero tambien quiero que implementes este plan /home/wal1rod@DIRMAD.COM.AR/projects/docs/nerv-deploy-registry-and-servers-plan-2026-10-26.md"
- **L2** Exploration: steps are built in `deployrun.Run`; target publish/deploy/verify and the host release run in `runProviders`; tools are checked in `loadProviders` (before clone); `deploy-ssh-compose` publish is `docker load -i {item}` over `image_tars` and `docker push {item}` over `images`; `deployconfig` parses strictly (unknown keys refused).
- **L3** (2026-10-26) Implemented in 73a6112: `internal/deployrun/publish.go` (core publish, docker credentials check from `auths`/`credHelpers`), step order push, publish, release, then targets; `deploy.publish` (*bool, default true); help, docs/deploy.md, docs/configuration.md, deploy skill, `/deploy:run`, `/deploy:setup`, deployer agent, ssh-compose README updated. Existing tests updated only where the spec changes the order (release before targets) or adds the `publish` step. Suite and vet green.
