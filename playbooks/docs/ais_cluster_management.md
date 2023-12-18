
# ais_cluster_management

## Purpose

Includes playbooks that help with:
1. Deploying AIS K8s operator
2. Deploy multi-node AIS cluster on K8s
3. Destroy AIS cluster
4. Cleanup AIS metadata and data
5. Undeploy AIS K8s operator

## Usage

### 1. Deploy AIS Operator

`ais_deploy_operator.yml` deploys the AIS K8s operator responsible for managing AIS cluster resource.

#### Args

`operator_version` (default: `v0.7`)- ais-k8s release version

#### Example

```console
$ ansible-playbook -i host.ini ais_deploy_operator.yml
```

### 2. Creating an AIS cluster

The `ais_deploy_cluster.yml` playbooks takes care of:
* Creating PVs for local persistent volumes
* Labeling K8s nodes to deploy AIS proxy/target pods
* Creating a K8s namespace (in which AIStore cluster will be deployed), and finally
* Deploying AIS Custom Resource

> **NOTE:** The playbook assumes the hostnames provided in the ansible inventory match the K8s node names. To override this, use the `node_name` option below.

#### Args

`ais_mpaths` - list of mountpaths on each node in cluster. Provide this variable by editing the `vars/ais_mpaths.yml` (refer to the example below) or using the CLI argument, e.g. `-e ais_mpaths=["/ais/sda", "/ais/sdb",...,"/ais/sdj"]`

`ais_mpath_size` - size of mountpath (eg. 9Ti, 512Gi, etc.)

```yaml
# example vars/ais_mpaths.yml
# ---
ais_mpaths:
  - "/ais/sda"
  - "/ais/sdb"
  - "/ais/sdc"
  - "/ais/sdd"
  - "/ais/sde"
  - "/ais/sdf"
  - "/ais/sdg"
  - "/ais/sdh"
  - "/ais/sdi"
  - "/ais/sdj"

ais_mpath_size: 9Ti

```

`node_image` (optional, e.g., `aistorage/aisnode:v3.21`) - docker image used by AIS target/proxy containers

`cluster` - specifies the ansible group to be used for deploying AIS cluster, eg.
```ini
# host.ini
...
[ais-1]
node-08
node-09
node-10
node-11
```

`node_name` (optional) - By default, the node labels and persistent volumes will be created assuming the hostnames provided in your ansible inventory **match the node names** in your K8s cluster. If this is not the case, you can specify the variable to use here. For example, to use the `ansible_host` variable containing the IP address of the node, set `-e node_name=ansible_host`.

#### Example

```console
$ ansible-playbook -i host.ini ais_deploy_cluster.yml -e cluster="ais-1"
```


### 3. Destroying an existing AIS cluster

The `ais_destroy_cluster.yml` playbooks:
* Destroys AIS cluster CR
* Deletes all the PVCs and PVs used by AIS targets
* Un-labels nodes

#### Args

`cluster` - same as above command

#### Example

```console
$ ansible-playbook -i host.ini ais_destroy_cluster.yml  -e cluster=ais-1
```

### 4. Cleanup markers and data

`ais_cleanup_all.yml` - cleanup all AIS Data and metadata on each node.

> ***WARNING:*** Deleted data cannot be restored!!!

#### Args:

`cluster` - same as above command

`ais_mpaths` - same as above command

#### Example

```console
$ ansible-playbook -i host.ini ais_cleanup_markers.yml -e cluster=ais-1
```


The `ais_cleanup_markers.yml` is responsible for deleting the `.ais.vmd` and `.ais.markers` present on each node in the group. Data stored on disks and BMD are preserved.

#### Args

`cluster` - same as above commands

`ais_mpaths` - same as above commands

#### Example

```console
$ ansible-playbook -i host.ini ais_cleanup_all.yml -e cluster=ais-1
```

### 5. Undeploy AIS Operator

`ais_undeploy_operator.yml` undeploy operator and delete all associated K8s resources

#### Example

```console
$ ansible-playbook -i host.ini ais_undeploy_operator.yml
```
