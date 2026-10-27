# Teamwork: `moveStage`

This implements the `moveStage` operation of the task contract (`task:task` skill §3).

`taskRef` `TW-<id>` is the Teamwork task id `<id>`: strip the prefix and the dash; the prefix is case-insensitive. The settings are `tasks.providers.teamwork.*`, read as the task skill's §1 says (the project file, plus the `scope: user` keys of the user file). Report every failure with the exact error text, and never retry.

1. **Target.** The target substring is `stages.<stageKey>`, falling back to the manifest's `stages`. Also accept any `stage_variants.<stageKey>` entries.
2. **Already there.** Read the task with `twprojects-get_task` (`fields: [workflowStages, tasklist]`). When its current stage's name contains the target (case- and accent-insensitive), return `{stage}` without moving it.
3. **Find the stage.** `twprojects-list_workflow_stages` for the task's workflow (`workflowStages[].workflowId`). Pick the first stage whose name contains the target.
4. **Move.** `twprojects-move_task_to_workflow_stage` with that `workflow_id`, the `stage_id` and `task_ids: [<id>]`.

When the task has no workflow, or no stage matches, the result is `skipped` with `no stage matching <target> in project <projectId>`. That is not a failure.
