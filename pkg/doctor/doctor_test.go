package doctor

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLintDirectory_UnprotectedDatabase(t *testing.T) {
	tempDir := t.TempDir()
	tfFile := filepath.Join(tempDir, "database.tf")

	content := `resource "aws_db_instance" "prod_db" {
  allocated_storage = 20
  engine            = "postgres"
  instance_class    = "db.t3.medium"
  name              = "proddb"
}
`
	if err := os.WriteFile(tfFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	report, err := LintDirectory(tempDir)
	if err != nil {
		t.Fatalf("lint failed: %v", err)
	}

	if report.StatefulResourcesFound != 1 {
		t.Errorf("expected 1 stateful resource, got %d", report.StatefulResourcesFound)
	}
	if report.UnprotectedResourcesFound != 1 {
		t.Errorf("expected 1 unprotected resource, got %d", report.UnprotectedResourcesFound)
	}
	if len(report.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(report.Findings))
	}
	if report.Findings[0].ResourceName != "prod_db" {
		t.Errorf("expected finding for prod_db, got %s", report.Findings[0].ResourceName)
	}
	if !report.Findings[0].MissingDeletionProtection {
		t.Errorf("expected missing deletion protection")
	}
}

func TestLintDirectory_ProtectedDatabase(t *testing.T) {
	tempDir := t.TempDir()
	tfFile := filepath.Join(tempDir, "database.tf")

	content := `resource "aws_db_instance" "hardened_db" {
  allocated_storage   = 100
  engine              = "postgres"
  deletion_protection = true

  lifecycle {
    prevent_destroy = true
  }
}
`
	if err := os.WriteFile(tfFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	report, err := LintDirectory(tempDir)
	if err != nil {
		t.Fatalf("lint failed: %v", err)
	}

	if report.StatefulResourcesFound != 1 {
		t.Errorf("expected 1 stateful resource, got %d", report.StatefulResourcesFound)
	}
	if report.UnprotectedResourcesFound != 0 {
		t.Errorf("expected 0 unprotected resources, got %d", report.UnprotectedResourcesFound)
	}
	if report.Severity != "HEALTHY" {
		t.Errorf("expected HEALTHY severity, got %s", report.Severity)
	}
}
