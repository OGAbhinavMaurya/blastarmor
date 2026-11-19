package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/OGAbhinavMaurya/blastarmor/pkg/advisor"
	"github.com/OGAbhinavMaurya/blastarmor/pkg/classifier"
	"github.com/OGAbhinavMaurya/blastarmor/pkg/doctor"
	"github.com/OGAbhinavMaurya/blastarmor/pkg/plan"
)

const version = "1.0.0"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]
	args := os.Args[2:]

	var err error
	switch command {
	case "check":
		err = runCheck(args)
	case "explain":
		err = runExplain(args)
	case "doctor":
		err = runDoctor(args)
	case "version", "-v", "--version":
		fmt.Printf("blastarmor version %s\n", version)
		return
	case "help", "-h", "--help":
		printUsage()
		return
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", command)
		printUsage()
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func printUsage() {
	usage := `blastarmor - Terraform & OpenTofu blast radius sentry and replacement circuit breaker

Usage:
  blastarmor <command> [options]

Commands:
  check      Intercept plan and break execution if unapproved stateful destruction is detected
  explain    Deeply explain the attributes forcing replacement and emit safe migration paths
  doctor     Scan local .tf files for stateful resources missing deletion protection
  version    Show blastarmor version
  help       Show this help message

Check Options:
  --plan               Path to Terraform plan JSON file (from terraform show -json) [required]
  --allow-stateless    Allow deletion/replacement of stateless compute resources [default: true]
  --break-glass-token  Explicit authorization token to bypass circuit breaker for intentional destruction
  --json               Output check results in JSON format

Explain Options:
  --plan               Path to Terraform plan JSON file [required]
  --address            Filter explanation to specific resource address (optional)
  --json               Output explanation in JSON format

Doctor Options:
  --dir                Root directory containing .tf configuration files [default: .]
  --json               Output doctor report in JSON format

Examples:
  terraform show -json tfplan.binary > plan.json
  blastarmor check --plan plan.json
  blastarmor explain --plan plan.json --address aws_db_instance.primary
  blastarmor doctor --dir ./terraform
`
	fmt.Print(usage)
}

type CheckReport struct {
	TotalDestructiveChanges int                        `json:"total_destructive_changes"`
	StatefulDestructions    int                        `json:"stateful_destructions"`
	StatelessDestructions   int                        `json:"stateless_destructions"`
	CircuitBreakerTripped   bool                       `json:"circuit_breaker_tripped"`
	BypassTokenProvided     bool                       `json:"bypass_token_provided"`
	BlockedChanges          []ClassifiedChange         `json:"blocked_changes"`
	AllowedChanges          []ClassifiedChange         `json:"allowed_changes"`
	Summary                 string                     `json:"summary"`
}

type ClassifiedChange struct {
	plan.DestructiveChange
	classifier.ClassificationResult
}

func runCheck(args []string) error {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	planPath := fs.String("plan", "", "Path to plan JSON file")
	allowStateless := fs.Bool("allow-stateless", true, "Allow stateless destruction")
	breakGlassToken := fs.String("break-glass-token", "", "Bypass token")
	jsonOutput := fs.Bool("json", false, "Output in JSON format")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if *planPath == "" {
		return fmt.Errorf("--plan argument is required (e.g. blastarmor check --plan plan.json)")
	}

	p, err := plan.ParsePlanFile(*planPath)
	if err != nil {
		return err
	}

	destructive := plan.ExtractDestructiveChanges(p)

	var blocked []ClassifiedChange
	var allowed []ClassifiedChange
	statefulCount := 0
	statelessCount := 0

	for _, d := range destructive {
		c := classifier.Classify(d.Type, d.ActionType)
		classified := ClassifiedChange{
			DestructiveChange:    d,
			ClassificationResult: c,
		}

		if c.IsStateful {
			statefulCount++
			blocked = append(blocked, classified)
		} else {
			statelessCount++
			if *allowStateless {
				allowed = append(allowed, classified)
			} else {
				blocked = append(blocked, classified)
			}
		}
	}

	tripped := len(blocked) > 0
	tokenValid := (*breakGlassToken == "PROD_OVERRIDE_ACKNOWLEDGED")

	var summary string
	if tripped && !tokenValid {
		summary = fmt.Sprintf("CIRCUIT BREAKER TRIPPED! Planned execution contains %d critical stateful resource destruction(s) or forced replacement(s). Execution blocked to prevent permanent data loss.", statefulCount)
	} else if tripped && tokenValid {
		summary = fmt.Sprintf("CIRCUIT BREAKER BYPASSED via break-glass token. %d stateful destruction(s) authorized.", statefulCount)
	} else {
		summary = "Plan check passed. Zero unapproved stateful resource deletions or forced replacements detected."
	}

	report := CheckReport{
		TotalDestructiveChanges: len(destructive),
		StatefulDestructions:    statefulCount,
		StatelessDestructions:   statelessCount,
		CircuitBreakerTripped:   tripped && !tokenValid,
		BypassTokenProvided:     tokenValid,
		BlockedChanges:          blocked,
		AllowedChanges:          allowed,
		Summary:                 summary,
	}

	if *jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(report)
	} else {
		fmt.Println("================================================================================")
		fmt.Println("                          BLASTARMOR PLAN CHECK REPORT                          ")
		fmt.Println("================================================================================")
		fmt.Printf(" Total Destructive Planned Changes: %d\n", report.TotalDestructiveChanges)
		fmt.Printf(" Stateful Resources Impacted:       %d (Data Loss Risk)\n", report.StatefulDestructions)
		fmt.Printf(" Stateless Resources Impacted:      %d\n", report.StatelessDestructions)
		fmt.Printf(" Circuit Breaker Status:            [%s]\n", getStatusBadge(report.CircuitBreakerTripped))
		fmt.Printf(" Summary:                           %s\n", report.Summary)
		fmt.Println("--------------------------------------------------------------------------------")

		if len(blocked) > 0 {
			fmt.Println(" Blocked Destructive Changes:")
			for _, b := range blocked {
				attrs := strings.Join(b.TriggeringAttributes, ", ")
				if attrs == "" {
					attrs = "none (direct deletion)"
				}
				fmt.Printf("   * [%s] %s (%s)\n", b.Severity, b.Address, b.ActionType)
				fmt.Printf("     Triggering Attributes: %s\n", attrs)
				fmt.Printf("     Risk: %s\n", b.RiskDescription)
			}
			fmt.Println("--------------------------------------------------------------------------------")
			fmt.Println(" Recommendation: Run 'blastarmor explain --plan plan.json' to review safe migration steps.")
		}
		fmt.Println("================================================================================")
	}

	if report.CircuitBreakerTripped {
		return fmt.Errorf("circuit breaker tripped: unapproved stateful destruction in plan (exit code 1)")
	}

	return nil
}

func getStatusBadge(tripped bool) string {
	if tripped {
		return "BLOCKED - ACTION REQUIRED"
	}
	return "PASSED - SAFE TO APPLY"
}

func runExplain(args []string) error {
	fs := flag.NewFlagSet("explain", flag.ContinueOnError)
	planPath := fs.String("plan", "", "Path to plan JSON file")
	addressFilter := fs.String("address", "", "Filter to specific address")
	jsonOutput := fs.Bool("json", false, "Output in JSON format")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if *planPath == "" {
		return fmt.Errorf("--plan argument is required (e.g. blastarmor explain --plan plan.json)")
	}

	p, err := plan.ParsePlanFile(*planPath)
	if err != nil {
		return err
	}

	destructive := plan.ExtractDestructiveChanges(p)
	var advices []advisor.MigrationAdvice

	for _, d := range destructive {
		if *addressFilter != "" && d.Address != *addressFilter {
			continue
		}
		advices = append(advices, advisor.GenerateAdvice(d))
	}

	if *jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(advices)
	}

	fmt.Println("================================================================================")
	fmt.Println("                        BLASTARMOR DESTRUCTIVE ACTION EXPLAINER                 ")
	fmt.Println("================================================================================")
	if len(advices) == 0 {
		fmt.Println(" Zero destructive changes match your criteria.")
		fmt.Println("================================================================================")
		return nil
	}

	for i, a := range advices {
		fmt.Printf(" [%d] %s (%s)\n", i+1, a.Address, a.ActionType)
		fmt.Printf("     Summary: %s\n", a.Summary)
		fmt.Printf("     Safe Migration Advice:\n     %s\n", a.RecommendedAction)
		if a.MovedBlock != "" {
			fmt.Printf("     Proposed Terraform 'moved' block:\n%s\n", a.MovedBlock)
		}
		if a.CLIMoveCommand != "" {
			fmt.Printf("     CLI Move Alternative: %s\n", a.CLIMoveCommand)
		}
		fmt.Println("--------------------------------------------------------------------------------")
	}
	fmt.Println("================================================================================")

	return nil
}

func runDoctor(args []string) error {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	dir := fs.String("dir", ".", "Directory containing .tf files")
	jsonOutput := fs.Bool("json", false, "Output in JSON format")

	if err := fs.Parse(args); err != nil {
		return err
	}

	report, err := doctor.LintDirectory(*dir)
	if err != nil {
		return err
	}

	if *jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	}

	fmt.Println("================================================================================")
	fmt.Println("                          BLASTARMOR CODEBASE LINTER REPORT                     ")
	fmt.Println("================================================================================")
	fmt.Printf(" Directory Scanned:         %s\n", *dir)
	fmt.Printf(" Total .tf Files Scanned:   %d\n", report.TotalFilesScanned)
	fmt.Printf(" Stateful Resources Found:  %d\n", report.StatefulResourcesFound)
	fmt.Printf(" Unprotected Resources:     %d\n", report.UnprotectedResourcesFound)
	fmt.Printf(" Severity:                  [%s]\n", report.Severity)
	fmt.Printf(" Summary:                   %s\n", report.Summary)
	fmt.Println("--------------------------------------------------------------------------------")

	if len(report.Findings) > 0 {
		fmt.Println(" Unprotected Stateful Resources:")
		for _, f := range report.Findings {
			fmt.Printf("   * %s:%d: resource %q %q\n", f.File, f.Line, f.ResourceType, f.ResourceName)
			fmt.Printf("     Recommendation: %s\n", f.Recommendation)
		}
		fmt.Println("--------------------------------------------------------------------------------")
	}
	fmt.Println("================================================================================")

	return nil
}
