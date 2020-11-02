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
> you can use `--ais` option to `./deploy.sh` script (see the following section).

### Deploy

Deployment consists of setting up the Kubernetes cluster on a specified cloud provider and deploying AIStore on the Kubernetes nodes.
`deploy.sh` is a one-place script that does everything for you.
If the script successfully finishes the AIStore cluster should be accessible and ready to be used.

To deploy just run `./deploy.sh --all` script and follow the instructions.

#### Supported arguments

| Flag | Description |
| ---- | ----------- |
| `--all` | Start nodes on specified provider, start K8s cluster and deploy AIStore on K8s nodes. |
| `--ais` | Only deploy AIStore on K8s nodes, assumes that K8s cluster is already deployed. |
| `--dashboard` | Start [K8s dashboard](https://kubernetes.io/docs/tasks/access-application-cluster/web-ui-dashboard) connected to started K8s cluster. |
| `--help` | Show help message. |

### Destroy

To remove and cleanup the cluster, we have created `destroy.sh --all` script.
Similarly, to the deploy script, it will walk you through required steps and the cleanup automatically.

#### Supported arguments

| Flag | Description |
| ---- | ----------- |
| `--all` | Stop AIStore Pods, and destroy started K8s nodes. |
| `--ais` | Only stop AIStore Pods so the cluster can be redeployed. |
| `--help` | Show help message. |

## Troubleshooting

### Google

> Error: googleapi: Error 403: Required '...' permission(s) for '...', forbidden

* Try to run:
    ```console
    $ gcloud auth application-default login
    ```
* Make sure `GOOGLE_APPLICATION_CREDENTIALS` is not set to credentials for other project/account.
* Make sure you have right permissions set for your account in [Google Console](https://console.cloud.google.com).
