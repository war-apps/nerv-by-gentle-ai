package nerv

import (
	"io/fs"
	"testing"
)

// A Teamwork task without a workflow has no stage to move to: moveStage
// records the stage change as a task comment instead, so the change is
// still visible on the task. A task whose workflow has no matching stage
// stays skipped.
func TestTeamworkMoveStage_CommentsWithoutAWorkflow(t *testing.T) {
	read := func(path string) string {
		data, err := fs.ReadFile(ExtensionsFS(), path)
		if err != nil {
			t.Fatalf("reading plugins/%s: %v", path, err)
		}
		return string(data)
	}
	assertContainsAll(t, "task-teamwork/procedures/move-stage.md", read("task-teamwork/procedures/move-stage.md"),
		"no workflow", "twprojects-create_comment", "comment_labels.stage", "ESTADO", "recorded as a comment",
		"no stage matching <target>")
	assertContainsAll(t, "task/skills/task/SKILL.md", read("task/skills/task/SKILL.md"),
		"`ESTADO`", "recorded as a comment")
}
