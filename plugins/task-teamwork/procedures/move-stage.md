# Teamwork: `moveStage`

This implements the `moveStage` operation of the task contract (`task:task` skill §3).

`taskRef` `TW-<id>` is the Teamwork task id `<id>`: strip the prefix and the dash; the prefix is case-insensitive. The settings are `tasks.providers.teamwork.*` from the merged `nerv.yaml`. Report every failure with the exact error text, and never retry.

1. **Target.** The target substring is `stages.<stageKey>`, falling back to the manifest's `stages`. Also accept any `stage_variants.<stageKey>` entries.
2. **Already there.** Read the task's current stage with `teamwork_get_task`. When its name contains the target (case- and accent-insensitive), return `{stage}` without moving it.
3. **Find the stage.** `teamwork_list_workflow_stages` for the task's project. Pick the first stage whose name contains the target.
4. **Move.** `teamwork_move_task_to_stage`, with the workflow id and the stage id.

When the project has no workflow, or no stage matches, the result is `skipped` with `no stage matching <target> in project <projectId>`. That is not a failure.
