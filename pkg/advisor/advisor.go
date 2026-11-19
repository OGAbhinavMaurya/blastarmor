package advisor

import (
	"fmt"
	"strings"

	"github.com/OGAbhinavMaurya/blastarmor/pkg/classifier"
	"github.com/OGAbhinavMaurya/blastarmor/pkg/plan"
)

// MigrationAdvice details safe, cloud-native migration steps to prevent downtime and data loss.
type MigrationAdvice struct {
	Address              string   `json:"address"`
	ResourceType         string   `json:"resource_type"`
	ActionType           string   `json:"action_type"`
	TriggeringAttributes []string `json:"triggering_attributes"`
	Summary              string   `json:"summary"`
	RecommendedAction    string   `json:"recommended_action"`
	MovedBlock           string   `json:"moved_block,omitempty"`
	CLIMoveCommand       string   `json:"cli_move_command,omitempty"`
}

// GenerateAdvice analyzes a destructive change and outputs non-destructive migration blueprints.
func GenerateAdvice(change plan.DestructiveChange) MigrationAdvice {
	isStateful := classifier.IsStateful(change.Type)
	attrsStr := strings.Join(change.TriggeringAttributes, ", ")

	advice := MigrationAdvice{
		Address:              change.Address,
		ResourceType:         change.Type,
		ActionType:           change.ActionType,
		TriggeringAttributes: change.TriggeringAttributes,
	}

	if change.IsReplacement {
		// Forces replacement scenario
		if isStateful {
			advice.Summary = fmt.Sprintf("Resource %s forces replacement due to changes in immutable attributes: [%s]. By default, Terraform will DESTROY the existing resource before creating a new one.", change.Address, attrsStr)
			if strings.Contains(change.Type, "db") || strings.Contains(change.Type, "sql") {
				advice.RecommendedAction = "Database immutable attribute change detected. Avoid direct in-place replacement: 1) Provision an updated Read Replica with the new configuration; 2) Allow replication to sync; 3) Promote the replica to standalone primary; 4) Re-route traffic endpoints; 5) Decommission the old database."
			} else if strings.Contains(change.Type, "bucket") {
				advice.RecommendedAction = "Bucket replacement will fail or cause data loss due to global S3 naming locks: 1) Create a secondary bucket with temporary suffix; 2) Enable replication or sync existing objects; 3) Update application write targets; 4) Drain and delete old bucket."
			} else {
				advice.RecommendedAction = "Provision replacement resource with a new identifier alongside the current resource, migrate state/traffic, and remove the old resource in a subsequent deployment."
			}
		} else {
			advice.Summary = fmt.Sprintf("Stateless resource %s is scheduled for replacement due to: [%s].", change.Address, attrsStr)
			advice.RecommendedAction = "Verify if create_before_destroy lifecycle rule is appropriate to minimize transient disruption."
		}
	} else if change.ActionType == "delete" {
		// Pure delete scenario
		if isStateful {
			advice.Summary = fmt.Sprintf("Stateful resource %s is scheduled for permanent deletion. If this resource was renamed or moved into a child module, Terraform will destroy the existing cloud instance and re-create it empty.", change.Address)
			advice.RecommendedAction = "If this resource was refactored or renamed, DO NOT run terraform apply. Add a 'moved' block in your Terraform configuration or run 'terraform state mv' to update the state address without cloud deletion."

			// Generate moved block sample
			advice.MovedBlock = fmt.Sprintf(`moved {
  from = %s
  to   = module.new_module.%s
}`, change.Address, change.Name)

			advice.CLIMoveCommand = fmt.Sprintf("terraform state mv %s module.new_module.%s", change.Address, change.Name)
		} else {
			advice.Summary = fmt.Sprintf("Resource %s is scheduled for deletion.", change.Address)
			advice.RecommendedAction = "Confirm that dependent services no longer require this resource."
		}
	}

	return advice
}
