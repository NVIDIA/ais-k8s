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
| Amazon (EKS) | `aws` | `aws` |
| Azure (AKS) | `azure` | |
| Google (GCP, GKE) | `gcp` | `gcloud` |


When using `deploy.sh` script you will be asked to specify provider ID.
Internally, the script will use the required commands - be sure you have them installed beforehand!

### Deploy

Deployment consists of setting up the Kubernetes cluster on specified cloud provider and deploying AIStore on the Kubernetes nodes.
`deploy.sh` is a one-place script that does everything for you.
After successful run the AIStore cluster should be accessible and ready to be used.

To deploy just run `./deploy.sh --all` script and follow the instructions.

#### Supported arguments

| Flag | Description |
| ---- | ----------- |
| `--all` | Starts nodes on specified provider, starts K8s cluster and deploys AIStore on K8s nodes. |
| `--ais` | Only deploy AIStore on K8s nodes, assumes that K8s cluster is already deployed. |
| `--dashboard` | Starts [K8s dashboard](https://kubernetes.io/docs/tasks/access-application-cluster/web-ui-dashboard) connected to started K8s cluster. |

### Destroy

To remove and cleanup the cluster, we have created `destroy.sh --all` script.
Similarly, to the deploy script, it will walk you through required steps and the cleanup automatically.

#### Supported arguments

| Flag | Description |
| ---- | ----------- |
| `--all` | Stops K8s pods, and destroys started nodes. |
| `--ais` | Only stops AIStore Pods so the cluster can be redeployed. |

## Troubleshooting

### Google

> Error: googleapi: Error 403: Required '...' permission(s) for '...', forbidden

1. Try to run:
```console
$ gcloud auth application-default login
```
2. Make sure `GOOGLE_APPLICATION_CREDENTIALS` is not set to credentials for other project/account.
3. Make sure you have right permissions set for your account in [Google Console](https://console.cloud.google.com).
