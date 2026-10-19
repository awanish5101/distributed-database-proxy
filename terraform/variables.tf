variable "primary_region" {
  type        = string
  default     = "us-east-1"
  description = "Primary AWS region"
}

variable "secondary_region" {
  type        = string
  default     = "us-west-2"
  description = "Cross-region replication AWS region"
}

variable "vpc_cidr" {
  type        = string
  default     = "10.0.0.0/16"
  description = "CIDR block for data infrastructure VPC"
}

variable "db_master_user" {
  type        = string
  default     = "infra_admin"
  description = "PostgreSQL cluster administrator user"
}

variable "db_master_password" {
  type        = string
  default     = "super_secure_vault_pw_2026!"
  sensitive   = true
  description = "Master password for PostgreSQL database cluster"
}
