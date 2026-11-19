package doctor

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/OGAbhinavMaurya/blastarmor/pkg/classifier"
)

var resourceHeaderRegex = regexp.MustCompile(`^\s*resource\s+"([^"]+)"\s+"([^"]+)"\s*\{`)

// LintFinding flags a stateful resource missing critical deletion protection guards.
type LintFinding struct {
	File                       string `json:"file"`
	Line                       int    `json:"line"`
	ResourceType               string `json:"resource_type"`
	ResourceName               string `json:"resource_name"`
	MissingDeletionProtection bool   `json:"missing_deletion_protection"`
	MissingPreventDestroy      bool   `json:"missing_prevent_destroy"`
	Recommendation             string `json:"recommendation"`
}

// LintReport aggregates static code analysis of local Terraform configurations.
type LintReport struct {
	TotalFilesScanned         int           `json:"total_files_scanned"`
	StatefulResourcesFound    int           `json:"stateful_resources_found"`
	UnprotectedResourcesFound int           `json:"unprotected_resources_found"`
	Findings                  []LintFinding `json:"findings"`
	Severity                  string        `json:"severity"` // "CRITICAL", "WARNING", "HEALTHY"
	Summary                   string        `json:"summary"`
}

// LintDirectory recursively inspects all .tf files in the given directory.
func LintDirectory(dirPath string) (*LintReport, error) {
	if dirPath == "" {
		dirPath = "."
	}

	report := &LintReport{
		Findings: make([]LintFinding, 0),
	}

	err := filepath.WalkDir(dirPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), ".") && d.Name() != "." {
				return filepath.SkipDir // Skip hidden directories like .terraform or .git
			}
			return nil
		}

		if strings.HasSuffix(d.Name(), ".tf") {
			report.TotalFilesScanned++
			findings, statefulCount, err := lintFile(path)
			if err != nil {
				return err
			}
			report.StatefulResourcesFound += statefulCount
			report.Findings = append(report.Findings, findings...)
		}
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("error linting directory %s: %w", dirPath, err)
	}

	report.UnprotectedResourcesFound = len(report.Findings)

	if report.UnprotectedResourcesFound > 0 {
		report.Severity = "WARNING"
		report.Summary = fmt.Sprintf("Found %d stateful resource(s) across %d files missing deletion_protection or prevent_destroy lifecycle guards.",
			report.UnprotectedResourcesFound, report.TotalFilesScanned)
	} else if report.StatefulResourcesFound > 0 {
		report.Severity = "HEALTHY"
		report.Summary = fmt.Sprintf("All %d stateful resource(s) across %d files have deletion protection or prevent_destroy configured.",
			report.StatefulResourcesFound, report.TotalFilesScanned)
	} else {
		report.Severity = "HEALTHY"
		report.Summary = fmt.Sprintf("Scanned %d file(s). No stateful resources detected.", report.TotalFilesScanned)
	}

	return report, nil
}

func lintFile(filePath string) ([]LintFinding, int, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, 0, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var findings []LintFinding
	statefulCount := 0

	inResource := false
	braceDepth := 0
	currentType := ""
	currentName := ""
	startLine := 0
	hasDeletionProtection := false
	hasPreventDestroy := false

	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if !inResource {
			matches := resourceHeaderRegex.FindStringSubmatch(line)
			if len(matches) == 3 {
				resType := matches[1]
				resName := matches[2]

				if classifier.IsStateful(resType) {
					inResource = true
					currentType = resType
					currentName = resName
					startLine = lineNum
					braceDepth = 1
					hasDeletionProtection = false
					hasPreventDestroy = false
					statefulCount++
				}
			}
		} else {
			// Inside stateful resource block
			braceDepth += strings.Count(line, "{")
			braceDepth -= strings.Count(line, "}")

			if strings.Contains(trimmed, "deletion_protection") && strings.Contains(trimmed, "true") {
				hasDeletionProtection = true
			}
			if strings.Contains(trimmed, "prevent_destroy") && strings.Contains(trimmed, "true") {
				hasPreventDestroy = true
			}

			if braceDepth <= 0 {
				// Block closed: evaluate findings
				missingProt := !hasDeletionProtection
				missingPrev := !hasPreventDestroy

				if missingProt || missingPrev {
					rec := fmt.Sprintf("Add 'deletion_protection = true' and 'lifecycle { prevent_destroy = true }' to %s.%s", currentType, currentName)
					findings = append(findings, LintFinding{
						File:                       filePath,
						Line:                       startLine,
						ResourceType:               currentType,
						ResourceName:               currentName,
						MissingDeletionProtection: missingProt,
						MissingPreventDestroy:      missingPrev,
						Recommendation:             rec,
					})
				}

				inResource = false
				currentType = ""
				currentName = ""
			}
		}
	}

	return findings, statefulCount, scanner.Err()
}
