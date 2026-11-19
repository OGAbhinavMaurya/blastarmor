package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMain_Usage(t *testing.T) {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	printUsage()

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	if !strings.Contains(output, "blastarmor - Terraform & OpenTofu blast radius sentry") {
		t.Fatalf("unexpected usage output: %s", output)
	}
	if !strings.Contains(output, "check") || !strings.Contains(output, "explain") || !strings.Contains(output, "doctor") {
		t.Fatalf("expected check, explain, and doctor in usage: %s", output)
	}
}

func TestRunCheck_DestructiveTripsCircuitBreaker(t *testing.T) {
	tempDir := t.TempDir()
	planPath := filepath.Join(tempDir, "plan.json")

	planJSON := `{
		"format_version": "1.2",
		"resource_changes": [
			{
				"address": "aws_db_instance.prod",
				"type": "aws_db_instance",
				"name": "prod",
				"change": {
					"actions": ["delete", "create"],
					"replace_paths": [["subnet_group_name"]]
				}
			}
		]
	}`
	if err := os.WriteFile(planPath, []byte(planJSON), 0644); err != nil {
		t.Fatalf("failed to write plan: %v", err)
	}

	// 1. Without token: must return error (exit code 1)
	err := runCheck([]string{"--plan", planPath})
	if err == nil {
		t.Fatalf("expected circuit breaker error on stateful replacement, got nil")
	}

	// 2. With break-glass token: must succeed
	err = runCheck([]string{"--plan", planPath, "--break-glass-token", "PROD_OVERRIDE_ACKNOWLEDGED"})
	if err != nil {
		t.Fatalf("expected bypass with valid token, got: %v", err)
	}
}

func TestRunExplain_Execution(t *testing.T) {
	tempDir := t.TempDir()
	planPath := filepath.Join(tempDir, "plan.json")

	planJSON := `{
		"format_version": "1.2",
		"resource_changes": [
			{
				"address": "aws_s3_bucket.data",
				"type": "aws_s3_bucket",
				"name": "data",
				"change": {
					"actions": ["delete"]
				}
			}
		]
	}`
	if err := os.WriteFile(planPath, []byte(planJSON), 0644); err != nil {
		t.Fatalf("failed to write plan: %v", err)
	}

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runExplain([]string{"--plan", planPath, "--json"})

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runExplain failed: %v", err)
	}

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	if !strings.Contains(output, "aws_s3_bucket.data") {
		t.Fatalf("expected s3 bucket address in explain json, got: %s", output)
	}
	if !strings.Contains(output, "moved {") {
		t.Fatalf("expected moved block in advice json, got: %s", output)
	}
}

func TestRunDoctor_Execution(t *testing.T) {
	tempDir := t.TempDir()
	tfPath := filepath.Join(tempDir, "main.tf")

	content := `resource "aws_dynamodb_table" "users" {
  name     = "users"
  hash_key = "id"
}
`
	if err := os.WriteFile(tfPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write tf file: %v", err)
	}

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runDoctor([]string{"--dir", tempDir, "--json"})

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runDoctor failed: %v", err)
	}

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	if !strings.Contains(output, "aws_dynamodb_table") {
		t.Fatalf("expected dynamodb finding in doctor json, got: %s", output)
	}
}

func TestRunCheck_InvalidFlag(t *testing.T) {
	err := runCheck([]string{"--unknown-flag-123"})
	if err == nil {
		t.Fatalf("expected error on invalid flag, got nil")
	}
}
