# nerv upgrade works on Windows

Objective: publish Windows binaries and let `nerv upgrade` replace a running `nerv.exe`, so Windows gets the same upgrade path as Linux and macOS (and a stamped `nerv version`).
Branch: `feat/windows-upgrade` from `develop`, one PR. Delivery: ask-on-risk; forecast ~300 authored lines. TDD: strict (session configuration); runner `go test ./...`.

## Specs

- **S1** (user, 2026-11-02) "porque no puedo hacer un upgrade en windows?" then "si" to implementing it.
- **S2** goreleaser builds `windows/amd64` and `windows/arm64`, packaged as `.zip` (`nerv_<version>_windows_<arch>.zip` containing `nerv.exe`); Linux and macOS archives stay `tar.gz` with unchanged names.
- **S3** `upgrade.CheckPlatform` accepts `windows`; `ArchiveName` returns the `.zip` name on Windows; the download and checksum verification work for it; extraction reads `nerv.exe` from the zip.
- **S4** Replacing a running `nerv.exe`: rename the current executable aside (`nerv.exe.old`), then move the new one into place; on failure, restore the original. A leftover `.old` is removed best-effort on a later run. Linux and macOS keep today's rename-over behavior.
- **S5** Docs: README, `docs/troubleshooting.md`, `docs/commands.md`, `docs/releases.md` (channels table: binaries), the upgrade usage text and the hint that pointed Windows to `go install`.
- **S6** Known environmental failures on this Windows machine (also on develop): cmd/nerv TestIntegration_Release*, TestUpgrade_StableReplacesBinaryAndRunsInstall; internal/artifact, atomicfile, deploy, deployrun, releaseplan, releaserun. CI (Linux) is the reference.

## Tasks

| ID | Specs | Route | Status | Commit |
|---|---|---|---|---|
| T1 | S2-S6 | delegated writer (.goreleaser.yaml, internal/upgrade, cmd/nerv, docs) | done: RED observed (internal/upgrade build failed on the new API), GREEN; vet/build clean; windows amd64/arm64 cross-build; parent: `goreleaser check` and snapshot with v2.15.2 (6 archives, windows zips contain nerv.exe) | 5b7d03d |

Route evidence: 2+ non-trivial Go files plus release config: writer trigger.

## Log

- **L1** (2026-11-02, user) Asked why Windows cannot upgrade; agreed to implement. No Teamwork task (same as the previous feature).
- **L2** Exploration: `.goreleaser.yaml` goos linux/darwin, tar.gz only; `internal/upgrade/upgrade.go` `CheckPlatform` refuses windows, `ArchiveName` is `.tar.gz`, `Replace`/`writeOver` gunzip + untar `nerv` and `os.Rename` over the running exe (blocked on Windows).
- **L3** (2026-11-02) T1 committed 5b7d03d: `Replace` takes goos; windows extracts `nerv.exe` from the zip and places it by renaming the running exe to `.old` (restored on failure); `CleanupStaleReplacement` removes a leftover `.old` at the start of `nerv upgrade`. Windows tests in cmd/nerv now use freebsd as the unsupported OS.
- **L4** (2026-11-02) Parent verification: goreleaser v2.15.2 (the Release workflow's version) `check` validates; `release --snapshot --skip=publish` produced linux/darwin tar.gz and windows amd64/arm64 zips with `nerv.exe`. Not verified: a real in-place upgrade on Windows, which needs a published release with the zips (first possible from the next release to the one that ships this).
