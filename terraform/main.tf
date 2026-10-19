terraform {
  required_version = ">= 1.5.0"
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
}

provider "aws" {
  region = var.primary_region
}

provider "aws" {
  alias  = "replica"
  region = var.secondary_region
}

# 1. Primary VPC & Networking
resource "aws_vpc" "data_infra_vpc" {
  cidr_block           = var.vpc_cidr
  enable_dns_hostnames = true
  enable_dns_support   = true

  tags = {
    Name        = "data-infra-primary-vpc"
    Environment = "production"
    ManagedBy   = "Terraform"
  }
}

resource "aws_subnet" "db_subnet_a" {
  vpc_id            = aws_vpc.data_infra_vpc.id
  cidr_block        = "10.0.1.0/24"
  availability_zone = "${var.primary_region}a"

  tags = {
    Name = "data-infra-db-subnet-a"
  }
}

resource "aws_subnet" "db_subnet_b" {
  vpc_id            = aws_vpc.data_infra_vpc.id
  cidr_block        = "10.0.2.0/24"
  availability_zone = "${var.primary_region}b"

  tags = {
    Name = "data-infra-db-subnet-b"
  }
}

resource "aws_db_subnet_group" "db_subnet_group" {
  name       = "data-infra-db-subnet-group"
  subnet_ids = [aws_subnet.db_subnet_a.id, aws_subnet.db_subnet_b.id]
}

# 2. Multi-Node Database Cluster Shards
resource "aws_rds_cluster" "shard_1" {
  cluster_identifier      = "db-shard-1-cluster"
  engine                  = "aurora-postgresql"
  engine_version          = "16.1"
  database_name           = "shard_1_prod"
  master_username         = var.db_master_user
  master_password         = var.db_master_password
  db_subnet_group_name    = aws_db_subnet_group.db_subnet_group.name
  skip_final_snapshot     = true
  deletion_protection     = false

  tags = {
    Role = "Shard-01"
  }
}

resource "aws_rds_cluster_instance" "shard_1_primary" {
  identifier         = "db-shard-1-primary"
  cluster_identifier = aws_rds_cluster.shard_1.id
  instance_class     = "db.r6g.large"
  engine             = aws_rds_cluster.shard_1.engine
}

resource "aws_rds_cluster_instance" "shard_1_replica" {
  identifier         = "db-shard-1-replica"
  cluster_identifier = aws_rds_cluster.shard_1.id
  instance_class     = "db.r6g.large"
  engine             = aws_rds_cluster.shard_1.engine
}

# 3. Distributed Cache Cluster (Redis)
resource "aws_elasticache_subnet_group" "redis_subnets" {
  name       = "data-infra-redis-subnet-group"
  subnet_ids = [aws_subnet.db_subnet_a.id, aws_subnet.db_subnet_b.id]
}

resource "aws_elasticache_replication_group" "proxy_cache" {
  replication_group_id          = "proxy-rate-limit-cache"
  description                   = "Distributed token bucket rate limiter and routing metadata"
  engine                        = "redis"
  node_type                     = "cache.m6g.large"
  num_cache_clusters            = 2
  parameter_group_name          = "default.redis7"
  port                          = 6379
  subnet_group_name             = aws_elasticache_subnet_group.redis_subnets.name
  automatic_failover_enabled    = true
}
