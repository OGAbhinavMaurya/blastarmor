package advisor

import (
	"strings"
	"testing"

	"github.com/OGAbhinavMaurya/blastarmor/pkg/plan"
)

func TestGenerateAdvice_DatabaseReplacement(t *testing.T) {
	change := plan.DestructiveChange{
		Address:              "aws_db_instance.primary",
		Type:                 "aws_db_instance",
		Name:                 "primary",
		ActionType:           "replace_destroy_then_create",
		IsReplacement:        true,
		TriggeringAttributes: []string{"subnet_group_name"},
	}

	advice := GenerateAdvice(change)
	if !strings.Contains(advice.Summary, "forces replacement") {
		t.Errorf("expected forces replacement summary, got: %s", advice.Summary)
	}
	if !strings.Contains(advice.RecommendedAction, "Read Replica") {
		t.Errorf("expected Read Replica recommendation, got: %s", advice.RecommendedAction)
	}
}

func TestGenerateAdvice_StatefulDeleteMovedBlock(t *testing.T) {
	change := plan.DestructiveChange{
		Address:    "aws_s3_bucket.user_uploads",
		Type:       "aws_s3_bucket",
		Name:       "user_uploads",
		ActionType: "delete",
	}

	advice := GenerateAdvice(change)
	if !strings.Contains(advice.Summary, "permanent deletion") {
		t.Errorf("expected permanent deletion warning, got: %s", advice.Summary)
	}
	if !strings.Contains(advice.MovedBlock, "moved {") {
		t.Errorf("expected moved block in advice, got: %s", advice.MovedBlock)
	}
	if !strings.Contains(advice.CLIMoveCommand, "terraform state mv") {
		t.Errorf("expected terraform state mv command, got: %s", advice.CLIMoveCommand)
	}
}
