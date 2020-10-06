output "kubernetes_cluster_name" {
  value       = google_container_cluster.primary.name
  description = "GKE AIStore cluster"
}

output "region" {
  value       = var.region
  description = "region"
}
