package plan

// Plan represents the JSON output format of terraform show -json tfplan.
type Plan struct {
	FormatVersion    string           `json:"format_version"`
	TerraformVersion string           `json:"terraform_version"`
	ResourceChanges  []ResourceChange `json:"resource_changes"`
}

// ResourceChange models a single resource difference planned by Terraform or OpenTofu.
type ResourceChange struct {
	Address       string `json:"address"`
	ModuleAddress string `json:"module_address,omitempty"`
	Mode          string `json:"mode"`
	Type          string `json:"type"`
	Name          string `json:"name"`
	ProviderName  string `json:"provider_name"`
	Change        Change `json:"change"`
}

// Change models the planned modifications, before/after values, and replacement triggers.
type Change struct {
	Actions      []string               `json:"actions"`
	Before       map[string]interface{} `json:"before"`
	After        map[string]interface{} `json:"after"`
	AfterUnknown map[string]interface{} `json:"after_unknown,omitempty"`
	ReplacePaths [][]interface{}        `json:"replace_paths,omitempty"`
}

// DestructiveChange isolates resources marked for deletion or replacement.
type DestructiveChange struct {
	Address              string                 `json:"address"`
	Type                 string                 `json:"type"`
	Name                 string                 `json:"name"`
	ActionType           string                 `json:"action_type"` // "delete", "replace_destroy_then_create", "replace_create_then_destroy"
	IsReplacement        bool                   `json:"is_replacement"`
	TriggeringAttributes []string               `json:"triggering_attributes"`
	BeforeValues         map[string]interface{} `json:"before_values,omitempty"`
	AfterValues          map[string]interface{} `json:"after_values,omitempty"`
}
