<div align="center">

# blastarmor

**Terraform and OpenTofu blast radius sentry, forces replacement circuit breaker, and stateful destruction guard.**

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/Go-1.22+-00ADD8?logo=go&logoColor=white)](go.mod)
[![Zero Dependencies](https://img.shields.io/badge/Dependencies-Zero%20(Pure%20Go)-green)](#)

<p align="center">
  <a href="#the-problem">The problem</a> •
  <a href="#how-it-works">How it works</a> •
  <a href="#architecture">Architecture</a> •
  <a href="#comparison">Comparison</a> •
  <a href="#installation">Installation</a> •
  <a href="#usage">Usage</a> •
  <a href="#license">License</a>
</p>

</div>

## The problem

In Terraform and OpenTofu, when an engineer modifies an attribute that cloud provider APIs cannot mutate in-place (such as an AWS RDS `db_subnet_group_name`, KMS key ARN, S3 bucket name, or GCP Cloud SQL tier), the engine marks the resource with `-/+` (forces replacement).

By default, Terraform replacement executes as destroy first, then create (`DeleteDBInstance`, `DeleteBucket`). In automated CI/CD pipelines or during manual reviews of 1,000-line plan outputs, cognitive fatigue obscures the destructive change:

```
  Engineer modifies .tf attribute            Terraform Execution Engine                      Cloud Provider (AWS / GCP / Azure)
                |                                        |                                                   |
  Alters RDS subnet_group_name                           |                                                   |
  ------------------------------------------------------>|  1. Computes plan diff                            |
                                                         |  Marks: -/+ (forces replacement)                  |
                                                         |  [Buried inside 1,200 lines of plan noise]        |
                                                         |                                                   |
  terraform apply -auto-approve                          |                                                   |
  ------------------------------------------------------>|  2. Executes replacement:                         |
                                                         |     DESTROY FIRST, THEN CREATE!                   |
                                                         |                                                   |
                                                         |  Issues DeleteDBInstance API call                 |
                                                         |-------------------------------------------------->|  Deletes production database!
                                                         |                                                   |  (Live data permanently lost!)
                                                         |                                                   |
                                                         |  Issues CreateDBInstance API call                 |
                                                         |-------------------------------------------------->|  Creates new empty database
                                                         |                                                   |  (Zero records, major outage)
```

This triggers severe production disasters:

1. **The destroy-before-create trap**: Terraform does not prompt for special confirmation when destroying a production database. If an attribute forces replacement, it issues synchronous delete API calls against the live infrastructure before attempting to create the new resource.
2. **The 1,000-line plan review blindness**: In real-world enterprise infrastructure, `terraform plan` frequently outputs 500 to 1,500 lines of harmless drift, tag updates, and security group adjustments. Reviewers skimming the output miss the single line that terminates the primary database.
3. **The false safety of native guards**:
   - `lifecycle { prevent_destroy = true }` only functions if the resource block exists in code. If an engineer refactors code, moves resources into a module, or renames a block without a `moved` block, Terraform bypasses `prevent_destroy` and permanently destroys the resource.
   - `create_before_destroy` fails on named databases, buckets, and virtual networks because cloud providers enforce global unique naming constraints.

### Why existing approaches fail

- **Manual code reviews**: Fatigue during pull request reviews guarantees that destructive replacement flags buried in lengthy plan outputs are overlooked.
- **Enterprise policy engines (OPA / Sentinel)**: Writing and maintaining custom Rego or Sentinel policies requires dedicated platform engineering teams and complex syntax parsing.
- **Native deletion protection**: Only protects specific resource types (such as RDS instances), leaving S3 buckets, DynamoDB tables, KMS keys, and EFS volumes unprotected against accidental destruction.

## How it works

`blastarmor` acts as an automated circuit breaker in CI/CD pipelines, intercepting planned destructive actions before `terraform apply` executes:

```
  CI/CD Pipeline (GitHub Actions / GitLab)           blastarmor Circuit Breaker                 Terraform Apply Gate
                     |                                           |                                       |
  terraform plan -out=tfplan.binary                              |                                       |
  terraform show -json tfplan.binary > plan.json                 |                                       |
  -------------------------------------------------------------->|                                       |
                                                                 |  1. Parses plan JSON                  |
                                                                 |  2. Isolates destructive actions      |
                                                                 |     (["delete"], ["delete", "create"])|
                                                                 |  3. Classifies stateful resources     |
                                                                 |  4. Discovers RDS forced replacement  |
                                                                 |                                       |
                                                                 |  [CIRCUIT BREAKER TRIPPED!]           |
                                                                 |  Outputs exact triggering attribute   |
                                                                 |  Emits safe read replica advice       |
                                                                 |  Exits with code 1                    |
                                                                 |                                       |
  Pipeline HALTS immediately!                                    |                                       |
  <--------------------------------------------------------------|                                       |
  (terraform apply is never executed; production database saved)                                         x (Blocked)
```

1. **Plan JSON inspection (`pkg/plan`)**: Parses execution plans (`terraform show -json`) using a pure Go parser. It identifies all destructive operations (`delete`, `replace_destroy_then_create`, and `replace_create_then_destroy`) and extracts the exact triggering attribute paths (`replace_paths`) that forced replacement.
2. **Stateful resource classification (`pkg/classifier`)**: Evaluates affected resources against a built-in registry of stateful cloud components (AWS RDS, Aurora, S3, DynamoDB, GCP Cloud SQL, BigQuery, Azure SQL, Cosmos DB, Kafka topics). Stateful destructions are scored as `CRITICAL`.
3. **Automated CI/CD circuit breaker (`cmd/blastarmor check`)**: Exits with code 1 if any unapproved stateful resource is marked for deletion or replacement, terminating the deployment pipeline before destructive API calls reach cloud providers.
4. **Migration advisor (`pkg/advisor`)**: Explains the exact reason replacement was triggered and generates non-destructive migration blueprints (such as read replica promotion patterns or `moved` blocks for refactored code).
5. **Pre-deployment codebase linter (`pkg/doctor`)**: Scans local `.tf` files to identify stateful resources missing `deletion_protection` or `prevent_destroy` declarations.

## Architecture

`blastarmor` is written in pure Go with zero external dependencies.

```
blastarmor/
├── cmd/
│   └── blastarmor/
│       ├── main.go             # CLI interface: check, explain, and doctor commands
│       └── main_test.go        # Command-line integration tests
├── pkg/
│   ├── plan/
│   │   ├── types.go            # Terraform execution plan JSON data structures
│   │   ├── parser.go           # Plan decoder and destructive action extractor
│   │   └── plan_test.go        # Plan parsing and replacement path extraction tests
│   ├── classifier/
│   │   ├── rules.go            # Stateful resource registry and severity scoring
│   │   └── classifier_test.go  # Resource classification tests
│   ├── advisor/
│   │   ├── advisor.go          # Non-destructive migration guidance and moved block generator
│   │   └── advisor_test.go     # Migration recommendation tests
│   └── doctor/
│       ├── linter.go           # Local .tf static analyzer checking deletion protection
│       └── doctor_test.go      # Codebase linter unit tests
├── go.mod
└── README.md
```

## Comparison

| Feature | Terraform Native | OPA / Conftest | Spacelift / TFC | blastarmor |
| :--- | :--- | :--- | :--- | :--- |
| **Circuit breaker exit code** | None | Requires Rego | SaaS Platform | Native binary (exit 1) |
| **Attribute replacement pinpointing** | Hidden in diff | Manual query | Basic diff | Automated path extraction |
| **Built-in stateful registry** | None | Manual rule lists | None | Pre-configured (AWS, GCP, Azure) |
| **Refactoring moved block generator** | None | None | None | Automated `moved` block output |
| **CI/CD pipeline integration** | Manual review | High complexity | Platform lock-in | Single CLI invocation |
| **Dependencies** | None | OPA binary + Rego | SaaS subscription | Pure Go (single binary) |

## Installation

### From source

```bash
git clone https://github.com/OGAbhinavMaurya/blastarmor.git
cd blastarmor
go build -o blastarmor ./cmd/blastarmor
```

### Pre-built binary

Download the compiled binary for your architecture from the GitHub releases page.

## Usage

### CI/CD circuit breaker mode

Add `blastarmor check` to your GitHub Actions or GitLab CI pipeline between `terraform plan` and `terraform apply`:

```bash
# Generate plan JSON
terraform plan -out=tfplan.binary
terraform show -json tfplan.binary > plan.json

# Run circuit breaker (exits code 1 on stateful destruction)
blastarmor check --plan plan.json
```

Example output when circuit breaker trips:

```text
================================================================================
                          BLASTARMOR PLAN CHECK REPORT                          
================================================================================
 Total Destructive Planned Changes: 2
 Stateful Resources Impacted:       1 (Data Loss Risk)
 Stateless Resources Impacted:      1
 Circuit Breaker Status:            [BLOCKED - ACTION REQUIRED]
 Summary:                           CIRCUIT BREAKER TRIPPED! Planned execution contains 1 critical stateful resource destruction(s) or forced replacement(s). Execution blocked to prevent permanent data loss.
--------------------------------------------------------------------------------
 Blocked Destructive Changes:
   * [CRITICAL] aws_db_instance.primary (replace_destroy_then_create)
     Triggering Attributes: db_subnet_group_name
     Risk: Relational database instance (RDS/Aurora) scheduled for deletion or forced replacement. Irreversible data loss hazard.
--------------------------------------------------------------------------------
 Recommendation: Run 'blastarmor explain --plan plan.json' to review safe migration steps.
================================================================================
```

### Explaining replacements and safe migrations

Run `blastarmor explain` to isolate triggering attributes and review safe alternative migration steps:

```bash
blastarmor explain --plan plan.json
```

Example output:

```text
================================================================================
                        BLASTARMOR DESTRUCTIVE ACTION EXPLAINER                 
================================================================================
 [1] aws_db_instance.primary (replace_destroy_then_create)
     Summary: Resource aws_db_instance.primary forces replacement due to changes in immutable attributes: [db_subnet_group_name]. By default, Terraform will DESTROY the existing resource before creating a new one.
     Safe Migration Advice:
     Database immutable attribute change detected. Avoid direct in-place replacement: 1) Provision an updated Read Replica with the new configuration; 2) Allow replication to sync; 3) Promote the replica to standalone primary; 4) Re-route traffic endpoints; 5) Decommission the old database.
--------------------------------------------------------------------------------
 [2] aws_s3_bucket.user_uploads (delete)
     Summary: Stateful resource aws_s3_bucket.user_uploads is scheduled for permanent deletion. If this resource was renamed or moved into a child module, Terraform will destroy the existing cloud instance and re-create it empty.
     Safe Migration Advice:
     If this resource was refactored or renamed, DO NOT run terraform apply. Add a 'moved' block in your Terraform configuration or run 'terraform state mv' to update the state address without cloud deletion.
     Proposed Terraform 'moved' block:
moved {
  from = aws_s3_bucket.user_uploads
  to   = module.new_module.user_uploads
}
     CLI Move Alternative: terraform state mv aws_s3_bucket.user_uploads module.new_module.user_uploads
--------------------------------------------------------------------------------
================================================================================
```

### Codebase linting mode

Scan local `.tf` files for stateful resources lacking deletion protection:

```bash
blastarmor doctor --dir ./terraform
```

Example output:

```text
================================================================================
                          BLASTARMOR CODEBASE LINTER REPORT                     
================================================================================
 Directory Scanned:         ./terraform
 Total .tf Files Scanned:   14
 Stateful Resources Found:  3
 Unprotected Resources:     1
 Severity:                  [WARNING]
 Summary:                   Found 1 stateful resource(s) across 14 files missing deletion_protection or prevent_destroy lifecycle guards.
--------------------------------------------------------------------------------
 Unprotected Stateful Resources:
   * terraform/rds.tf:12: resource "aws_db_instance" "analytics_db"
     Recommendation: Add 'deletion_protection = true' and 'lifecycle { prevent_destroy = true }' to aws_db_instance.analytics_db
--------------------------------------------------------------------------------
================================================================================
```

## License

This project is licensed under the MIT License. See the [LICENSE](LICENSE) file for details.
