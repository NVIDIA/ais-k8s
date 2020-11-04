output "kubernetes_cluster_name" {
  value       = google_container_cluster.primary.name
  description = "Name of GKE cluster"
}

output "region" {
  value       = var.region
  description = "Region where the cluster was deployed"
}

output "external_ip" {
  value       = google_compute_address.static.address
  description = "External IP to access the AIStore cluster"
}
