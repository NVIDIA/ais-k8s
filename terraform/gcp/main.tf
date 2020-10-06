provider "google" {
  project = var.project_id
  region  = var.region
}

variable "gke_username" {
  default     = ""
  description = "GKE username"
}

variable "gke_password" {
  default     = ""
  description = "GKE password"
}

variable "gke_num_nodes" {
  default     = 1
  description = "number of GKE nodes"
}

variable "project_id" {
  description = "project id"
}

variable "region" {
  type        = string
  default     = "us-central1"
  description = "region"
}

# GKE cluster.
resource "google_container_cluster" "primary" {
  name     = "ais"
  location = var.region

  remove_default_node_pool = true
  initial_node_count       = 1

  network    = google_compute_network.vpc.name
  subnetwork = google_compute_subnetwork.subnet.name

  master_auth {
    username = var.gke_username
    password = var.gke_password

    client_certificate_config {
      issue_client_certificate = false
    }
  }
}

# Separately managed node pool.
resource "google_container_node_pool" "primary_nodes" {
  name       = "${google_container_cluster.primary.name}-node-pool"
  location   = var.region
  cluster    = google_container_cluster.primary.name
  node_count = var.gke_num_nodes

  node_config {
    oauth_scopes = [
      "https://www.googleapis.com/auth/logging.write",
      "https://www.googleapis.com/auth/monitoring",
    ]

    labels = {
      env = var.project_id
    }

    preemptible     = true # IMPORTANT: Lowers price approximately 3 times.
    machine_type    = "n1-standard-1" # 1vCPU + 3.75GB MEM
    image_type      = "COS" # TODO: Change to some Ubuntu version.
    # disk_type       = "" # TODO: set the disk type.
    disk_size_gb    = 40 # Single 50GB disk each node.
    local_ssd_count = 0

    tags     = ["ais-node", "ais"]
    metadata = {
      disable-legacy-endpoints = "true"
    }
  }
}
