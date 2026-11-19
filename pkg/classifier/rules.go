package classifier

import (
	"strings"
)

// Resource categories
const (
	CategoryStatefulData     = "StatefulData"     // Databases, S3 buckets, disks, KMS keys (Permanent data loss risk)
	CategoryNetworkingIAM    = "NetworkingIAM"    // VPCs, subnets, IAM roles (Downtime & security blast radius)
	CategoryStatelessCompute = "StatelessCompute" // Ephemeral VMs, launch templates, security group rules
	CategoryUnknown          = "Unknown"
)

// Severity levels
const (
	SeverityCritical = "CRITICAL" // Automatic CI/CD circuit breaker trip (exit code 1)
	SeverityHigh     = "HIGH"
	SeverityMedium   = "MEDIUM"
	SeverityLow      = "LOW"
)

var statefulResourcePrefixes = map[string]string{
	// AWS Stateful Resources
	"aws_db_instance":                    "Relational database instance (RDS/Aurora)",
	"aws_rds_cluster":                    "Aurora database cluster",
	"aws_rds_cluster_instance":           "Aurora database cluster instance",
	"aws_s3_bucket":                      "S3 object storage bucket",
	"aws_dynamodb_table":                 "DynamoDB NoSQL database table",
	"aws_elasticache_replication_group": "ElastiCache Redis replication group",
	"aws_elasticache_cluster":            "ElastiCache cache cluster",
	"aws_efs_file_system":                "EFS distributed network file system",
	"aws_ebs_volume":                     "EBS persistent block storage volume",
	"aws_kms_key":                        "KMS cryptographic master key",
	"aws_secretsmanager_secret":          "Secrets Manager secret vault",

	// GCP Stateful Resources
	"google_sql_database_instance": "Cloud SQL database instance",
	"google_storage_bucket":        "Cloud Storage bucket",
	"google_bigquery_dataset":      "BigQuery data warehouse dataset",
	"google_kms_crypto_key":        "Cloud KMS encryption key",
	"google_spanner_instance":      "Cloud Spanner globally distributed database",
	"google_redis_instance":        "Memorystore Redis cluster",

	// Azure Stateful Resources
	"azurerm_mssql_database":    "Azure SQL Database",
	"azurerm_mssql_server":      "Azure SQL Server",
	"azurerm_storage_account":   "Azure Storage Account (Blob/Table)",
	"azurerm_cosmosdb_account":  "Azure Cosmos DB Account",
	"azurerm_redis_cache":       "Azure Cache for Redis",
	"azurerm_key_vault":         "Azure Key Vault",

	// Generic & Third-party
	"kafka_topic":                       "Kafka / Redpanda message streaming topic",
	"vault_generic_secret":              "HashiCorp Vault secret",
	"postgresql_database":               "PostgreSQL database",
	"mysql_database":                    "MySQL database",
	"kubernetes_persistent_volume_claim": "Kubernetes Persistent Volume Claim (PVC)",
}

var networkingIAMPrefixes = map[string]string{
	"aws_vpc":                 "VPC Network",
	"aws_subnet":              "Subnet",
	"aws_internet_gateway":    "Internet Gateway",
	"aws_nat_gateway":         "NAT Gateway",
	"aws_route_table":         "Route Table",
	"aws_iam_role":            "IAM Role",
	"aws_iam_policy":          "IAM Policy",
	"google_compute_network":  "VPC Network",
	"azurerm_virtual_network": "Virtual Network",
}

// ClassificationResult details the risk assessment of a planned destructive change.
type ClassificationResult struct {
	Category        string `json:"category"`
	Severity        string `json:"severity"`
	IsStateful      bool   `json:"is_stateful"`
	RiskDescription string `json:"risk_description"`
}

// Classify categorizes a resource and assigns an operational severity level.
func Classify(resType string, actionType string) ClassificationResult {
	cleanType := strings.TrimSpace(resType)

	// Check if stateful resource
	if desc, ok := statefulResourcePrefixes[cleanType]; ok {
		risk := desc + " scheduled for deletion or forced replacement. Irreversible data loss hazard."
		return ClassificationResult{
			Category:        CategoryStatefulData,
			Severity:        SeverityCritical,
			IsStateful:      true,
			RiskDescription: risk,
		}
	}

	// Check if networking / IAM
	if desc, ok := networkingIAMPrefixes[cleanType]; ok {
		risk := desc + " modification may cause widespread connectivity drops or authentication lockouts."
		return ClassificationResult{
			Category:        CategoryNetworkingIAM,
			Severity:        SeverityHigh,
			IsStateful:      false,
			RiskDescription: risk,
		}
	}

	// Stateless compute / ephemeral
	risk := "Stateless resource replacement or deletion."
	sev := SeverityMedium
	if actionType == "delete" {
		sev = SeverityLow
	}

	return ClassificationResult{
		Category:        CategoryStatelessCompute,
		Severity:        sev,
		IsStateful:      false,
		RiskDescription: risk,
	}
}

// IsStateful checks if a resource type carries persistent data loss risk.
func IsStateful(resType string) bool {
	_, ok := statefulResourcePrefixes[strings.TrimSpace(resType)]
	return ok
}
