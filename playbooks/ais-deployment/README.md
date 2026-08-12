> **Deprecation Notice**:
>
> Deployment with Ansible Playbooks is no longer actively supported.
>
> Refer to the [documentation for recommended deployment options](../../docs/README.md#aistore-deployment)


The playbooks in this directory operate on the mountpaths and node placement of a deployed AIStore cluster.

Each playbook is documented separately. For a full walkthrough of deployment, see the [deployment guide](../../docs/README.md).

| Playbook(s)                                                | Description                                                                                                                                                                                                                                                                                                                                                           |
|------------------------------------------------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| [ais_cleanup_all](ais_cleanup_all.yml)                     | Clean up all metadata AND data from the cluster.                                                                                                                                                                                                                                                                                                                      |
| [ais_cleanup_markers](ais_cleanup_markers.yml)             | Clean up metadata and markers on targets.                                                                                                                                                                                                                                                                                                                             |
| [ais_replace_node](ais_replace_node.yml)                   | Automates replacing a K8s node hosting ais-target and ais-proxy pods in the AIS (AIStore) cluster. It is used to move these workloads to another node, typically for maintenance or decommissioning. WARNING: Destructive, if not properly managed.                                                                                                                   |
