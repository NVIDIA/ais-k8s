# Playbooks

This directory contains ansible playbooks for setting up an AIStore cluster in K8s.

## Prerequisites

1. Ansible installed locally
  See the [Ansible installation guide](https://docs.ansible.com/ansible/latest/installation_guide/intro_installation.html).

## Getting Started

The playbooks are broken up into multiple sections, which should be executed in order. 

1. [host-config](./host-config/README.md) playbooks configure K8s nodes to optimize the network and storage performance
2. [cloud](./cloud/README.md) playbooks set up K8s secrets with static credentials for accessing cloud backends, e.g. s3 and gcp
3. (optional) [security](./security/README.md) contains the [`os-hardening` playbook](security/os_hardening.yaml) to harden the OS for CISCAT scans. This includes various security measures such as configuring sysctl settings, journald, sshd, and ensuring audit logs and AIDE setup.
4. [**DEPRECATED -- Use [Helm Charts](../helm)**] [ais-deployment](./ais-deployment/README.md) playbooks configure resources in the AIS namespace including the AIStore custom resource, managed by the operator

An example hosts file is provided, [hosts-example.ini](./hosts-example.ini). You will need to set this up with your own hosts before running the playbooks.
Make sure to specify the `controller` node in the `hosts.ini` file and configure the controller host with `kubectl` access.

### Quick Setup

> **Deprecation Notice**:
> 
> Deployment with Ansible Playbooks is no longer actively supported.
>
> Refer to the [documentation for recommended deployment options](../docs/README.md#aistore-deployment) 


Before deploying with the playbook below, the AIS K8s Operator must be installed.
See the [operator installation docs](../docs/README.md#operator-deployment) for instructions.

To deploy the AIS Cluster:
  ```bash
  ansible-playbook -i hosts.ini ais_deploy.yml -e cluster=ais
  ```

## Scaling the AIS Cluster

### Adding New Nodes to the AIS Cluster

To integrate new nodes into your cluster:

1. **Configure New Hosts**: First, add any new nodes to the [hosts.ini](./hosts-example.ini) file under a `new_nodes` group, providing the necessary host details.
    ```ini
    ...
    [new_nodes]
    new_worker ansible_host=x.x.x.x
    ...
    ```

> Note: You can skip step 2 if your nodes are already setup correctly for AIS.

2. **Run the Host Configuration Playbook**: Execute the `ais_host_config.yml` playbook targeting the `new_nodes` group to configure the new hosts.
    ```bash
    ansible-playbook -i hosts.ini ais_host_config.yml -e ais_hosts=new_nodes
    ```

3. **Update the Hosts File**: After setting up the new hosts, include the `new_nodes` group under `[ais:children]` in the [hosts.ini](./hosts-example.ini) file. **NOTE:** Ensure that the `new_nodes` group is added at the bottom of the to list to avoid interference with the existing setup.
    ```ini
    [ais:children]
    controller
    new_nodes
    ```

4. **Deploy the Cluster**: With the new nodes configured, run the `ais_deploy_cluster` playbook to update your AIS cluster.
    ```console
    ansible-playbook -i hosts.ini ais-deployment/ais_deploy_cluster.yml -e cluster=ais
    ```

### Downscaling the AIS Cluster

To decrease the number of nodes in your current AIS Cluster, use this playbook -
  ```
  ansible-playbook -i hosts.ini ais-deployment/ais_downscale_cluster.yml -e size=X
  ```

For additional ansible config tweaks, you can create an `ansible.cfg` file. Check the [Ansible documentation](https://docs.ansible.com/ansible/latest/installation_guide/intro_configuration.html) for this, as options may change with new versions. 