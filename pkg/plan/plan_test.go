package plan

import (
	"strings"
	"testing"
)

func TestParsePlan_ExtractDestructiveChanges(t *testing.T) {
	planJSON := `{
		"format_version": "1.2",
		"terraform_version": "1.6.0",
		"resource_changes": [
			{
				"address": "aws_security_group_rule.allow_http",
				"type": "aws_security_group_rule",
				"name": "allow_http",
				"change": {
					"actions": ["update"],
					"before": {"description": "old"},
					"after": {"description": "new"}
				}
			},
			{
				"address": "aws_s3_bucket.logs",
				"type": "aws_s3_bucket",
				"name": "logs",
				"change": {
					"actions": ["delete"],
					"before": {"bucket": "prod-logs"}
				}
			},
			{
				"address": "aws_db_instance.primary",
				"type": "aws_db_instance",
				"name": "primary",
				"change": {
					"actions": ["delete", "create"],
					"before": {"id": "db-123", "db_subnet_group_name": "subnet-a"},
					"after": {"id": "db-123", "db_subnet_group_name": "subnet-b"},
					"replace_paths": [["db_subnet_group_name"]]
				}
			}
		]
	}`

	p, err := ParsePlan(strings.NewReader(planJSON))
	if err != nil {
		t.Fatalf("failed to parse plan: %v", err)
	}

	if len(p.ResourceChanges) != 3 {
		t.Fatalf("expected 3 resource changes, got %d", len(p.ResourceChanges))
	}

	destructive := ExtractDestructiveChanges(p)
	if len(destructive) != 2 {
		t.Fatalf("expected 2 destructive changes, got %d", len(destructive))
	}

	// 1. S3 bucket delete
	if destructive[0].Address != "aws_s3_bucket.logs" || destructive[0].ActionType != "delete" {
		t.Errorf("unexpected first destructive change: %+v", destructive[0])
	}
	if destructive[0].IsReplacement {
		t.Errorf("s3 bucket should be pure delete, not replacement")
	}

	// 2. RDS instance replace_destroy_then_create
	if destructive[1].Address != "aws_db_instance.primary" {
		t.Errorf("unexpected second destructive change address: %s", destructive[1].Address)
	}
	if destructive[1].ActionType != "replace_destroy_then_create" {
		t.Errorf("expected replace_destroy_then_create, got %s", destructive[1].ActionType)
	}
	if !destructive[1].IsReplacement {
		t.Errorf("rds should be marked replacement")
	}
	if len(destructive[1].TriggeringAttributes) != 1 || destructive[1].TriggeringAttributes[0] != "db_subnet_group_name" {
		t.Errorf("expected triggering attribute db_subnet_group_name, got %+v", destructive[1].TriggeringAttributes)
	}
}

func TestParsePlan_InvalidJSON(t *testing.T) {
	_, err := ParsePlan(strings.NewReader(`{invalid json`))
	if err == nil {
		t.Fatalf("expected error on invalid json, got nil")
	}
}
