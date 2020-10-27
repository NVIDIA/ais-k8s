// TODO: uncomment when we figure out problem with ssh and VPC
//# VPC
//resource "google_compute_network" "vpc" {
//  name                    = "ais-vpc"
//  auto_create_subnetworks = "false"
//}
//
//# Subnet
//resource "google_compute_subnetwork" "subnet" {
//  name          = "ais-subnet"
//  region        = var.region
//  network       = google_compute_network.vpc.name
//  ip_cidr_range = "10.10.0.0/24"
//
//  private_ip_google_access = true
//}
