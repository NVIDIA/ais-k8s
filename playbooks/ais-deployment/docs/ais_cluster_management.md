
# AIS Cluster Management

Collection of Ansible playbooks designed for efficient management of AIStore (AIS) clusters in Kubernetes environments.

## Overview

The playbooks assist in the following tasks:

1. **Deploying AIS Kubernetes Operator:** Set up the AIS K8s operator, responsible for managing AIS cluster resources.
1. **Creating a Multi-Node AIS Cluster:** Facilitate the deployment of a multi-node AIS cluster on Kubernetes.
1. **AIS Cluster Shutdown:** Guidelines for gracefully shutting down an AIS cluster.
1. **AIS Cluster Decommission:** Steps for decommissioning and permanently removing an AIS cluster.
1. **Cleanup of AIS Data and Metadata:** Tools for removing AIS data and metadata from the cluster nodes.
1. **Undeploying AIS Kubernetes Operator:** Steps to undeploy the AIS K8s operator and delete associated Kubernetes resources.

> **Note:** For comprehensive details on different cluster lifecycle operations, please visit the [AIS documentation](https://github.com/NVIDIA/aistore/blob/main/docs/lifecycle_node.md).

## Detailed Usage

### 1. Deploying AIS Kubernetes Operator

- **Playbook:** [`ais_deploy_operator.yml`](../ais_deploy_operator.yml)
- **Purpose:** Deploys the AIS K8s operator to manage AIS cluster resources.
- **Default Operator Version:** `v2.8.0` (modifiable in [defaults/main.yml](../roles/ais_deploy_operator/defaults/main.yml)). Refer to our [compatibility matrix](../../../docs/COMPATIBILITY.md) for supported versions.
  - Modify the version specified in the [defaults/main.yml](../roles/ais_deploy_operator/defaults/main.yml) to match the desired version.
- **Operator Releases:** [GitHub Releases](https://github.com/NVIDIA/ais-k8s/releases)
- **Execution Example:**
  ```console
  $ ansible-playbook -i host.ini ais_deploy_operator.yml
  ```

### 2. Deploying AIStore

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
    
    - `node_image`: Specify the Docker image for AIS target/proxy containers (e.g., `aistorage/aisnode:v3.23`). Find the latest image at the [AIS Docker Hub repository](https://hub.docker.com/r/aistorage/aisnode/tags). Refer to our [compatibility matrix](../../../docs/COMPATIBILITY.md) for supported versions.
    - `gcp_secret_name`/`aws_secret_name`: For cloud backend integration, create a Kubernetes secret with the necessary credentials as described in this [cloud credentials playbook](../../cloud/README.md).
    - `protocol`: Choose between 'http' or 'https'. For 'https', you'll need to create the required certificate by following the[`https configuration doc`](../../ais-deployment/docs/ais_https_configuration.md).
    - `proxy_size`, `target_size`: Number of proxy and target pods you want to deploy in your cluster. Note: 0 < `proxy_size`, `target_size` <= `cluster_size`

- **Optional Arguments:**

`node_name` (optional) - Specify if hostnames in the Ansible inventory do not match Kubernetes node names. e.g. to use `ansible_host` variable containing the IP address of the node, set `-e node_name=ansible_host`.

- **Execution Example:**
  ```console
  $ ansible-playbook -i host.ini ais_deploy_cluster.yml -e cluster=ais
  ```

### 3. AIS Cluster Shutdown

- **Playbook:** [`ais_shutdown_cluster.yml`](../ais_shutdown_cluster.yml)
- **Overview:**
  - Gracefully shuts down an AIS cluster, preserving metadata and configuration for future restarts.
- **Parameters:** `cluster` (identifier used during cluster setup)
- **Usage:**
  ```console
  $ ansible-playbook -i host.ini ais_shutdown_cluster.yml -e cluster=ais
  ```

### 4. AIS Cluster Decommission

- **Playbook:** [`ais_decommission_cluster.yml`](../ais_decommission_cluster.yml)
- **Overview:**
  - Cleans up the AIS cluster's resources, including cluster maps, configuration files, PVCs, PVs, and node labels.
  - Ensures a complete removal of the AIS cluster from the Kubernetes environment.
- **Parameters:** `cluster` (identifier used during cluster setup)
- **Usage:**
  ```console
  $ ansible-playbook -i host.ini ais_decommission_cluster.yml -e cluster=ais
  ```

### 5. Cleanup of AIS Data and Metadata

- **Playbook:** [`ais_cleanup_all.yml`](../ais_cleanup_all.yml)
- **Purpose:** Complete removal of AIS data and metadata from each node.
- **Warning:** This action is irreversible!
- **Key Arguments:** `cluster`, `ais_mpaths` (same as used in cluster creation)
- **Execution Example:**
  ```console
  $ ansible-playbook -i host.ini ais_cleanup_all.yml -e cluster=ais
  ```

### 6. Undeploying AIS Kubernetes Operator

- **Playbook:** [`ais_undeploy_operator.yml`](../ais_undeploy_operator.yml)
- **Action:** Removes the AIS operator and all associated Kubernetes resources.
- **Execution Example:**
  ```console
  $ ansible-playbook -i host.ini ais_undeploy_operator.yml
  ```