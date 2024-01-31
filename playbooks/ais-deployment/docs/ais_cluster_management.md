
# AIS Cluster Management

Collection of Ansible playbooks designed for efficient management of AIStore (AIS) clusters in Kubernetes environments.

## Overview

The playbooks assist in the following tasks:

1. **Deploying AIS Kubernetes Operator:** Set up the AIS K8s operator, responsible for managing AIS cluster resources.
2. **Creating a Multi-Node AIS Cluster:** Facilitate the deployment of a multi-node AIS cluster on Kubernetes.
3. **Decommissioning an AIS Cluster:** Procedures for safely destroying an existing AIS cluster.
4. **Cleanup of AIS Data and Metadata:** Tools for removing AIS data and metadata from the cluster nodes.
5. **Undeploying AIS Kubernetes Operator:** Steps to undeploy the AIS K8s operator and delete associated Kubernetes resources.

## Detailed Usage

### 1. Deploying AIS Kubernetes Operator

- **Playbook:** [`ais_deploy_operator.yml`](../ais_deploy_operator.yml)
- **Purpose:** Deploys the AIS K8s operator to manage AIS cluster resources.
- **Default Operator Version:** `v0.97` (modifiable in [defaults/main.yml](../roles/ais_deploy_operator/defaults/main.yml))
- **Operator Releases:** [GitHub Releases](https://github.com/NVIDIA/ais-k8s/releases)
- **Execution Example:**
  ```console
  $ ansible-playbook -i host.ini ais_deploy_operator.yml
  ```

### 2. Creating a Multi-Node AIS Cluster

- **Playbook:** [`ais_deploy_cluster.yml`](../ais_deploy_cluster.yml)
- **Tasks:**
  - Creation of persistent volumes (PVs).
  - Labeling Kubernetes nodes for AIS proxy/target pods.
  - Setting up a dedicated Kubernetes namespace.
  - Deploying AIS Custom Resource.
- **Key Arguments:**
  - `ais_mpaths`: List of mount paths on each node (e.g., `["/ais/sda", "/ais/sdb", ...]`).
  - `ais_mpath_size`: Size of each mount path (e.g., `9Ti`, `512Gi`).

  > Note: Provide these variable by editing the [`vars/ais_mpaths.yml`](../vars/ais_mpaths.yml) (refer to the example below) or using the CLI argument, e.g. `-e ais_mpaths=["/ais/sda", "/ais/sdb",...,"/ais/sdj"]`
  
    **Device Configuration Example:**
    Consider this example output from `lsblk`:
    ```
    NAME        MAJ:MIN RM  SIZE RO TYPE MOUNTPOINT
    ...
    nvme0n1     259:12   0  6.2T  0 disk /ais/nvme0n1
    ...
    nvme7n1     259:6    0  6.2T  0 disk /ais/nvme7n1
    ```
    Based on this, your configuration in `ais_mpaths.yml` would be:
    ```yaml
    ais_mpaths:
      - "/ais/nvme0n1"
      ...
      - "/ais/nvme7n1"

    ais_mpath_size: 6.2Ti
    ```

- **Playbook Defaults**:
    
    In the [defaults file](../roles/ais_deploy_cluster/defaults/main.yml) for the deploy cluster playbook, update values such as:
    
    - `node_image`: Specify the Docker image for AIS target/proxy containers (e.g., `aistorage/aisnode:v3.21`). Find the latest image at the [AIS Docker Hub repository](https://hub.docker.com/r/aistorage/aisnode/tags).
    - `gcp_secret_name`/`aws_secret_name`: For cloud backend integration, create a Kubernetes secret with the necessary credentials as described in this [cloud credentials playbook](../../cloud/README.md).
    - Protocol: Choose between 'http' or 'https'. For 'https', you'll need to create the required certificate by following the[`https configuration doc`](../../ais-deployment/docs/https_configuration.md).

- **Optional Arguments:**

`node_name` (optional) - Specify if hostnames in the Ansible inventory do not match Kubernetes node names. e.g. to use `ansible_host` variable containing the IP address of the node, set `-e node_name=ansible_host`.

- **Execution Example:**
  ```console
  $ ansible-playbook -i host.ini ais_deploy_cluster.yml -e cluster=ais
  ```

### 3. Decommissioning an Existing AIS Cluster

- **Playbook:** [`ais_destroy_cluster.yml`](../ais_destroy_cluster.yml)
- **Actions:**
  - Deletion of cluster map and configuration files.
  - Destruction of AIS cluster Custom Resource.
  - Deletion of PVCs and PVs used by AIS targets.
  - Removal of node labels.
- **Arguments:** `cluster` (same as used in creation)
- **Execution Example:**
  ```console
  $ ansible-playbook -i host.ini ais_destroy_cluster.yml -e cluster=ais
  ```

### 4. Cleanup of AIS Data and Metadata

- **Playbook:** [`ais_cleanup_all.yml`](../ais_cleanup_all.yml)
- **Purpose:** Complete removal of AIS data and metadata from each node.
- **Warning:** This action is irreversible!
- **Key Arguments:** `cluster`, `ais_mpaths` (same as used in cluster creation)
- **Execution Example:**
  ```console
  $ ansible-playbook -i host.ini ais_cleanup_all.yml -e cluster=ais-1
  ```

### 5. Undeploying AIS Kubernetes Operator

- **Playbook:** [`ais_undeploy_operator.yml`](../ais_undeploy_operator.yml)
- **Action:** Removes the AIS operator and all associated Kubernetes resources.
- **Execution Example:**
  ```console
  $ ansible-playbook -i host.ini ais_undeploy_operator.yml
  ```