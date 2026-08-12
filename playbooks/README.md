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

An example hosts file is provided, [hosts-example.ini](./hosts-example.ini). You will need to set this up with your own hosts before running the playbooks.
Make sure to specify the `controller` node in the `hosts.ini` file and configure the controller host with `kubectl` access.

For additional ansible config tweaks, you can create an `ansible.cfg` file. Check the [Ansible documentation](https://docs.ansible.com/ansible/latest/installation_guide/intro_configuration.html) for this, as options may change with new versions. 