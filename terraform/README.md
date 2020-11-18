## Start AIStore cluster on the cloud

This directory contains Terraform files and scripts that allow deploying AIStore cluster on the Kubernetes in the cloud.
These main script `deploy.sh` will walk you through required steps to set up the AIStore cluster.

Note that in this tutorial we expect that you have `terraform`, `kubectl` and `helm` commands installed.
The Terraform is used to deploy the Kubernetes on specified cloud provider and `kubectl`/`helm` are used for deploying the AIStore.

### Cloud providers

The cluster will be deployed on one of the supported cloud providers.
Below you can check which cloud providers are supported and what is required to use them.

| Provider | ID | Required Commands |
| -------- | --- | ----------------- |
| Google (GCP, GKE) | `gcp` | `gcloud` |

When using `deploy.sh` script you will be asked to specify cloud provider ID.
Internally, the script will use the required commands - be sure you have them installed beforehand!

> If you already have a running Kubernetes cluster, regardless of a cluster provider,
> you can use `ais` argument to `./deploy.sh` script (see the following section).

#### Google

In `gcp/main.tf` file you can find a couple of variables that can be adjusted to your preferences:
* `zone` - zone in which the cluster will be deployed (for now it's only possible to deploy cluster on a single zone; using [regional cluster](https://cloud.google.com/kubernetes-engine/docs/concepts/types-of-clusters#regional_clusters) is not yet supported).
* `machine_type` - machine type which will be used as GKE nodes (see [full list](https://cloud.google.com/compute/docs/machine-types)).
* `machine_preemptible` - determines if the machine is preemptible (more info [here](https://cloud.google.com/compute/docs/instances/preemptible)).

### Deploy

Deployment consists of setting up the Kubernetes cluster on a specified cloud provider and deploying AIStore on the Kubernetes nodes.
`deploy.sh` is a one-place script that does everything for you.
If the script successfully finishes the AIStore cluster should be accessible and ready to be used.

To deploy just run `./deploy.sh all` script and follow the instructions.

#### Supported arguments

`./deploy.sh DEPLOY_TYPE [--flag=value ...]`

There are 3 `DEPLOY_TYPE`s:
* `all` - start nodes on specified provider, start K8s cluster and deploy AIStore on K8s nodes.
* `ais` - only deploy AIStore on K8s nodes, assumes that K8s cluster is already deployed.
* `dashboard` - start [K8s dashboard](https://kubernetes.io/docs/tasks/access-application-cluster/web-ui-dashboard) connected to started K8s cluster.

| Flag | Description |
| ---- | ----------- |
| `--cloud` | Cloud provider to be used (`aws`, `azure` or `gcp`). |
| `--node-cnt` | Number of instances/nodes to be started. |
| `--disk-cnt` | Number of disks per instance/node. |
| `--cluster-name` | Name of the Kubernetes cluster. |
| `--help` | Show help message. |

#### Admin container

After full deployment you should be able to list all K8s Pods:
```console
$ ./deploy.sh all --cloud=gcp --node-cnt=2 --disk-cnt=2
...
$ kubectl get pods
NAME                   READY   STATUS    RESTARTS   AGE
demo-ais-admin-99p8r   1/1     Running   0          31m
demo-ais-proxy-5vqb8   1/1     Running   0          31m
demo-ais-proxy-g7jf7   1/1     Running   0          31m
demo-ais-target-0      1/1     Running   0          31m
demo-ais-target-1      1/1     Running   0          29m
```

As you can see there is one special Pod called `demo-ais-admin-*`.
It contains useful binaries:
 * `ais` (more [here](github.com/NVIDIA/aistore/cmd/cli/README.md)),
 * `aisloader` (more [here](github.com/NVIDIA/aistore/bench/aisloader/README.md)),
 * `xmeta` (more [here](github.com/NVIDIA/aistore/cmd/xmeta/README.md)).

Thanks to them you can access and stress-load the cluster.

After logging into the container, the commands are already configured to point to the deployed cluster:
```console
$ kubectl exec -it demo-ais-admin-99p8r -- /bin/bash
root@demo-ais-admin-99p8r:/#
root@demo-ais-admin-99p8r:/# ais show cluster
PROXY		 MEM USED %	 MEM AVAIL	 CPU USED %	 UPTIME	 STATUS
rOFMYYks	 0.79		 3.60GiB	 0.00		 49m	 healthy
zloxzvzK[P]	 0.82		 3.60GiB	 0.00		 50m	 healthy

TARGET		 MEM USED %	 MEM AVAIL	 CAP USED %	 CAP AVAIL	 CPU USED %	 REBALANCE		 UPTIME	 STATUS
BEtMbslT	 0.83		 3.60GiB	 0		 99.789GiB	 0.00		 finished; 0 moved (0B)	 49m	 healthy
MbXeFcFw	 0.84		 3.60GiB	 0		 99.789GiB	 0.00		 finished; 0 moved (0B)	 48m	 healthy

Summary:
 Proxies:	2 (0 - unelectable)
 Targets:	2
 Primary Proxy:	zloxzvzK
 Smap Version:	8
```

### Destroy

To remove and cleanup the cluster, we have created `destroy.sh all` script.
Similarly, to the deploy script, it will walk you through required steps and the cleanup automatically.

#### Supported arguments

`./destroy.sh DESTROY_TYPE [--flag=value ...]`

There are 2 `DESTROY_TYPE`s:
* `all` - stop AIStore Pods, and destroy started K8s nodes.
* `ais` - only stop AIStore Pods, so the cluster can be redeployed.

| Flag | Description |
| ---- | ----------- |
| `--preserve-disks` | Do not remove persistent volumes - data on targets. It will be available on the next deployment. Not supported with `all` |
| `--help` | Show help message. |

## Troubleshooting

### Google

> googleapi: Error 403: Required '...' permission(s) for '...', forbidden

This may happen when an account does not have enough permissions to create or access specific resource on GCP.

What you can do:
* Try to run:
    ```console
    $ gcloud auth application-default login
    ```
* Make sure `GOOGLE_APPLICATION_CREDENTIALS` is not set to credentials for other project/account.
* Make sure you have right permissions set for your account in [Google Console](https://console.cloud.google.com).

> googleapi: Error 403: Insufficient regional quota to satisfy request ...

This may happen if an account's or project's quota is exceeded.
Running the cluster on a free account can easily exceed available resources (for free account we recommend 2-4 nodes and 1-3 disks per node).

What you can do:
* Try to run the cluster with lower number of nodes (something between 2-4).
* Try to run the cluster with lower number of disks per node (something between 1-3).
* Try to increase the quote on [GCP Console](https://console.cloud.google.com/iam-admin/quotas).
