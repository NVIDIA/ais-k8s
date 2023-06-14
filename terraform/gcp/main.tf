provider "google" {
  project = var.project_id
  zone    = var.zone
}

# Deployment specific variables.

locals {
  image_type = "ubuntu_containerd"
}

variable "cluster_name" {
  type        = string
  default     = "ais"
  description = "Name of the cluster"
}

variable "ais_release_name" {
  type        = string
  description = "AIS release name, matching Helm variable"
}

# GCP/GKE specific variables.

variable "project_id" {
  type        = string
  description = "GCP project ID"
}

variable "user" {
  type        = string
  description = "GCP username"
}

variable "ssh-key" {
  type        = string
  default     = "~/.ssh/id_rsa.pub"
  description = "SSH public key path"
}

# Cluster specific variables.

variable "zone" {
  type        = string
  default     = "us-central1-a"
  description = "Zone where the cluster should be deployed, see: https://cloud.google.com/kubernetes-engine/docs/concepts/types-of-clusters"
}

variable "machine_type" {
  type        = string
  default     = "n1-standard-1"
  description = "Type of machines cluster will be deployed on, see: https://cloud.google.com/compute/docs/machine-types"
}

variable "machine_preemptible" {
  type        = bool
  default     = true
  description = "Determines if the machine should be preemptible or not, see: https://cloud.google.com/compute/docs/instances/preemptible"
}

variable "node_count" {
  type        = number
  description = "Number of GKE nodes"
}

# GKE cluster.
resource "google_container_cluster" "primary" {
  name     = var.cluster_name
  location = var.zone

  remove_default_node_pool = true
  initial_node_count       = 1

  # TODO: Uncomment when we are able to run VPC + ssh.
  # network    = google_compute_network.vpc.name

  master_auth {
    client_certificate_config {
      issue_client_certificate = false
    }
  }
}


# Separately managed node pool.
resource "google_container_node_pool" "primary_nodes" {
  name       = "${google_container_cluster.primary.name}-node-pool"
  location   = var.zone
  cluster    = google_container_cluster.primary.name
  node_count = var.node_count

  node_config {
    oauth_scopes = [
      "https://www.googleapis.com/auth/compute",
      "https://www.googleapis.com/auth/logging.write",
      "https://www.googleapis.com/auth/monitoring",
    ]

    labels = {
      env = var.project_id

      "nvidia.com/ais-target" = "${var.ais_release_name}-ais"
      "nvidia.com/ais-proxy"  = "${var.ais_release_name}-ais-electable"
    }

    preemptible  = var.machine_preemptible # IMPORTANT: Lowers price approximately 3 times.
    machine_type = var.machine_type # 1vCPU + 3.75GB MEM
    image_type   = local.image_type

    tags     = [var.cluster_name]
    metadata = {
      disable-legacy-endpoints = "true"
      enable-guest-attributes  = "true"

      ssh-keys = fileexists(var.ssh-key) ? "ais:${file(var.ssh-key)}" : ""
    }
  }
}

# Static IP
resource "google_compute_address" "static" {
  name = "${google_container_cluster.primary.name}-external"
}
