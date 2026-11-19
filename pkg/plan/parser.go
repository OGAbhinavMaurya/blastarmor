package plan

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

// ParsePlan decodes a Terraform execution plan from an io.Reader.
func ParsePlan(r io.Reader) (*Plan, error) {
	var plan Plan
	dec := json.NewDecoder(r)
	if err := dec.Decode(&plan); err != nil {
		return nil, fmt.Errorf("failed to decode terraform plan JSON: %w", err)
	}
	return &plan, nil
}

// ParsePlanFile reads and decodes a Terraform execution plan from a JSON file path.
func ParsePlanFile(filePath string) (*Plan, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open plan file %s: %w", filePath, err)
	}
	defer file.Close()

	return ParsePlan(file)
}

// ExtractDestructiveChanges identifies all resources scheduled for deletion or forces replacement.
func ExtractDestructiveChanges(plan *Plan) []DestructiveChange {
	if plan == nil {
		return nil
	}

	results := make([]DestructiveChange, 0)

	for _, rc := range plan.ResourceChanges {
		actions := rc.Change.Actions
		hasDelete := false
		for _, a := range actions {
			if a == "delete" {
				hasDelete = true
				break
			}
		}

		if !hasDelete {
			continue
		}

		var actionType string
		isReplacement := false

		if len(actions) == 1 && actions[0] == "delete" {
			actionType = "delete"
		} else if len(actions) == 2 {
			isReplacement = true
			if actions[0] == "delete" && actions[1] == "create" {
				actionType = "replace_destroy_then_create"
			} else if actions[0] == "create" && actions[1] == "delete" {
				actionType = "replace_create_then_destroy"
			} else {
				actionType = "replace"
			}
		} else {
			actionType = strings.Join(actions, "_")
		}

		// Extract triggering attributes from replace_paths
		triggeringAttrs := make([]string, 0)
		for _, pathElements := range rc.Change.ReplacePaths {
			parts := make([]string, len(pathElements))
			for i, el := range pathElements {
				parts[i] = fmt.Sprintf("%v", el)
			}
			triggeringAttrs = append(triggeringAttrs, strings.Join(parts, "."))
		}

		// If replace_paths is empty on replacement, fallback to diffing before vs after
		if isReplacement && len(triggeringAttrs) == 0 && rc.Change.Before != nil && rc.Change.After != nil {
			for k, vBefore := range rc.Change.Before {
				vAfter, exists := rc.Change.After[k]
				if exists && fmt.Sprintf("%v", vBefore) != fmt.Sprintf("%v", vAfter) {
					triggeringAttrs = append(triggeringAttrs, k)
				}
			}
		}

		results = append(results, DestructiveChange{
			Address:              rc.Address,
			Type:                 rc.Type,
			Name:                 rc.Name,
			ActionType:           actionType,
			IsReplacement:        isReplacement,
			TriggeringAttributes: triggeringAttrs,
			BeforeValues:         rc.Change.Before,
			AfterValues:          rc.Change.After,
		})
	}

	return results
}
