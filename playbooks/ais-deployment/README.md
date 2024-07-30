The playbooks in this directory provide a simple way to control the deployment of an AIStore cluster and its related resources. 

Each playbook is documented separately. For a full walkthrough of deployment, see the [deployment guide](../../docs/README.md).

Playbook(s) | Description
----------- | -----------
[ais_cleanup_all](ais_cleanup_all.yml) | Clean up all metadata AND data from the cluster.
[ais_cleanup_markers](ais_cleanup_markers.yml) | Clean up metadata and markers on targets.
[ais_deploy_cluster](ais_deploy_cluster.yml) | Deploy an AIS cluster. See [ais_cluster_management docs](docs/ais_cluster_management.md).
[ais_deploy_operator](ais_deploy_operator.yml)| Deploy the operator which enables cluster dpeloyment. See [ais_cluster_management docs](docs/ais_cluster_management.md).
[ais_shutdown_cluster](ais_shutdown_cluster.yml) | Gracefully shuts down an AIS cluster, preserving metadata and configuration for future restarts.
[ais_decommission_cluster](ais_decommission_cluster.yml) | Cleans up the AIS cluster's resources, including cluster maps, configuration files, PVCs, PVs, and node labels. Ensures a complete removal of the AIS cluster from the K8s env.
[ais_switch_protocol](ais_switch_protocol.yml) | Switch between http/https clusters. See [switch_protocol docs](docs/switch_protocol.md).
[ais_undeploy_operator](ais_undeploy_operator.yml) | Remove the operator from the K8s cluster.
[create_network_definition](create_network_definition.yml) | Create network definitions for multihome deployments. See [multihome docs](docs/deploy_with_multihome.md).
[fetch_ca_cert](fetch_ca_cert.yml) | Fetch the CA cert secret for a client to use self-signed certificate verification. See [ais_https_configuration docs](docs/ais_https_configuration.md)
[generate_https_cert](generate_https_cert.yml) | Generate HTTPS certificates for the cluster. See [generate_https_cert docs](docs/generate_https_cert.md).
[install_requirements.yml](install_requirements.yml) | Install required ansible collections locally and Python requirements on the K8s controller host.
[ais_downscale_cluster](ais_downscale_cluster.yml) | Decrease the number of nodes (proxy and target) in your current AIS Cluster. See [scaling docs](../README.md#downscaling-the-ais-cluster).
[ais_deploy_authn](ais_deploy_authn.yml) | Deploy the AIStore Authentication (AuthN) server.
[ais_undeploy_authn](ais_undeploy_authn.yml) | Undeploy the AIStore Authentication (AuthN) server.