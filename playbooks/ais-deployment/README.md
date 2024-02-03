The playbooks in this directory provide a simple way to control the deployment of an AIStore cluster and its related resources. 

Each playbook is documented separately. For a full walkthrough of deployment, see the [deployment guide](../../docs/README.md).

Playbook(s) | Description
----------- | -----------
[ais_cleanup_all](ais_cleanup_all.yml) | Clean up all metadata AND data from the cluster.
[ais_cleanup_markers](ais_cleanup_markers.yml) | Cleaning up metadata and markers on targets.
[ais_deploy_cluster](ais_deploy_cluster.yml) | Deploy an AIS cluster. See [ais_cluster_management docs](docs/ais_cluster_management.md).
[ais_deploy_operator](ais_deploy_operator.yml)| Deploy the operator which enables cluster dpeloyment. See [ais_cluster_management docs](docs/ais_cluster_management.md).
[ais_destroy_cluster](ais_destroy_cluster.yml) | Destroy the cluster, prompting for additional cleanup.
[fetch_ca_cert](fetch_ca_cert.yml) | Fetch the CA cert secret for a client to use self-signed certificate verification. See [ais_https_configuration docs](docs/ais_https_configuration.md)
[generate_https_cert](generate_https_cert.yml) | Generate HTTPS certificates for the cluster. See [generate_https_cert docs](docs/generate_https_cert.md).
[ais_switch_protocol](ais_switch_protocol.md) | Used for switching between http/https clusters. See [switch_protocol docs](docs/switch_protocol.md).
[ais_undeploy_operator](ais_undeploy_operator.yml) | Remove the operator from the K8s cluster.
[create_network_definition](create_network_definition.yml) | Create network definitions for multihome deployments. See [multihome docs](docs/deploy_with_multihome.md).