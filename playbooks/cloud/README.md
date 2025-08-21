# Cloud Bucket Configuration Playbooks

This directory contains playbooks for managing configuration updates related to cloud buckets, specifically for AWS and GCP environments.

---
## AWS Configuration

Steps to modify AWS access for an AIStore cluster within Kubernetes:

1. Place the required AWS `config` and `credentials` files into the directory [`roles/aws_config/files`](roles/aws_config/files).
2. Execute the AWS playbook using the command: `ansible-playbook -i inventory.yaml playbooks/cloud/ais_aws_config.yml`.
3. The playbook will transfer the AWS configuration files to the Kubernetes controller host. It will then create or update the Kubernetes secret and delete the files from the local system.

## GCP Configuration

Steps to modify GCP access for an AIStore cluster within Kubernetes:

1. Place the required GCP `credentials` JSON file (`gcp.json`) into the directory [`roles/gcp_config/files`](roles/gcp_config/files).
2. Execute the GCP playbook using the command: `ansible-playbook -i inventory.yaml playbooks/cloud/ais_gcp_config.yml`.
3. The playbook will transfer the GCP credentials JSON to the Kubernetes controller host. It will then create or update the Kubernetes secret and delete the files from the local system.

## OCI Configuration

Steps to modify OCI access for an AIStore cluster within Kubernetes:

1. Place the required OCI private key file (`oci_api_key`) into the directory [`roles/oci_config/files`](roles/oci_config/files).
2. Update the OCI configuration variables in [`vars/oci_config.yml`](vars/oci_config.yml) with your:
   - `oci_tenancy_ocid`: Your OCI tenancy OCID
   - `oci_user_ocid`: Your OCI user OCID  
   - `oci_region`: Your OCI region (e.g., `us-chicago-1`)
   - `oci_fingerprint`: Your API key fingerprint
   - `oci_compartment_ocid`: Your OCI compartment OCID
3. Execute the OCI playbook using the command: `ansible-playbook -i inventory.yaml playbooks/cloud/ais_oci_config.yml`.
4. The playbook will transfer the OCI credentials and configuration to the Kubernetes controller host. It will then create or update the Kubernetes secret and delete the files from the local system.

**Note:** Ensure that the AIStore cluster is pre-configured for AWS, GCP, or OCI integration and possesses an existing cloud provider Kubernetes secret. Refer to the [operator deployment instructions](../../operator/README.md) for additional information.