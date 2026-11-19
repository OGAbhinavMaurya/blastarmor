package classifier

import (
	"testing"
)

func TestClassify_StatefulResources(t *testing.T) {
	statefulTypes := []string{
		"aws_db_instance",
		"aws_s3_bucket",
		"aws_dynamodb_table",
		"google_sql_database_instance",
		"google_storage_bucket",
		"azurerm_mssql_database",
		"kafka_topic",
	}

	for _, resType := range statefulTypes {
		res := Classify(resType, "replace_destroy_then_create")
		if !res.IsStateful {
			t.Errorf("expected %s to be stateful", resType)
		}
		if res.Category != CategoryStatefulData {
			t.Errorf("expected CategoryStatefulData for %s, got %s", resType, res.Category)
		}
		if res.Severity != SeverityCritical {
			t.Errorf("expected SeverityCritical for %s, got %s", resType, res.Severity)
		}
	}
}

func TestClassify_NetworkingIAM(t *testing.T) {
	res := Classify("aws_vpc", "delete")
	if res.Category != CategoryNetworkingIAM {
		t.Errorf("expected CategoryNetworkingIAM, got %s", res.Category)
	}
	if res.Severity != SeverityHigh {
		t.Errorf("expected SeverityHigh, got %s", res.Severity)
	}
	if res.IsStateful {
		t.Errorf("aws_vpc should not be classified as stateful data")
	}
}

func TestClassify_StatelessCompute(t *testing.T) {
	res := Classify("aws_security_group_rule", "replace_destroy_then_create")
	if res.Category != CategoryStatelessCompute {
		t.Errorf("expected CategoryStatelessCompute, got %s", res.Category)
	}
	if res.Severity != SeverityMedium {
		t.Errorf("expected SeverityMedium for stateless replacement, got %s", res.Severity)
	}
}
