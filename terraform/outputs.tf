output "vpc_id" {
  value       = aws_vpc.data_infra_vpc.id
  description = "Data Infrastructure VPC ID"
}

output "shard_1_primary_endpoint" {
  value       = aws_rds_cluster.shard_1.endpoint
  description = "Primary writer endpoint for Shard 1"
}

output "shard_1_reader_endpoint" {
  value       = aws_rds_cluster.shard_1.reader_endpoint
  description = "Read replica load-balanced endpoint for Shard 1"
}

output "redis_primary_endpoint" {
  value       = aws_elasticache_replication_group.proxy_cache.primary_endpoint_address
  description = "Distributed Redis cache primary endpoint"
}
