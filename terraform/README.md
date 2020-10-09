## Start AIStore cluster on the cloud

This directory contains Terraform files and scripts that allow deploying AIStore cluster on the Kubernetes in the cloud.
These main script `deploy.sh` will walk you through required steps to set up the AIStore cluster.

Note that in this tutorial we expect that you have `terraform` and `kubectl` commands installed.
The Terraform is used to deploy the Kubernetes on specified cloud provider and `kubectl` for deploying the AIStore.


### Cloud providers

The cluster will be deployed on one of the supported cloud providers.
Below you can check which cloud providers are supported and what is required to use them.


| Provider | ID | Required Commands |
| -------- | --- | ----------------- |
| Amazon (EKS) | `aws` | `aws` |
| Azure (AKS) | `azure` | |
| Google (GCP, GKE) | `gcp` | `gcloud` |


When using `deploy.sh` script you will be asked to specify provider ID.
Internally, the script will use the required commands so be sure you have them installed beforehand.

### Deploy

Deployment consists of setting up the Kubernetes cluster on specified cloud provider and deploying AIStore on the Kubernetes nodes.
`deploy.sh` is a one-place script that does everything for you.
After successful run the AIStore cluster should be accessible and ready to be used.

To deploy just run `./deploy.sh --all` script and follow the instructions.

### Destroy

To remove and cleanup the cluster, we have created `destroy.sh --all` script.
Similarly, to the deploy script, it will walk you through required steps and the cleanup automatically.


### Example

Let's try to run `deploy.sh` script on Google Cloud.

```console
$ ./deploy.sh --all
Select cloud provider (aws, azure, gcp): gcp

Initializing the backend...

Initializing provider plugins...
- Using previously-installed hashicorp/google v3.42.0

The following providers do not have any version constraints in configuration,
so the latest version was installed.

To prevent automatic upgrades to new major versions that may contain breaking
changes, we recommend adding version constraints in a required_providers block
in your configuration, with the constraint strings suggested below.

* hashicorp/google: version = "~> 3.42.0"

Terraform has been successfully initialized!

<TODO>
```


## Troubleshooting

### Google

> Error: Error creating Network: googleapi: Error 403: Required 'compute.networks.create' permission for '...', forbidden

1. Try to run:
```console
$ gcloud auth application-default login
```
2. Make sure `GOOGLE_APPLICATION_CREDENTIALS` is not set to credentials for other project/account.
3. Make sure you have right permissions set for your account in [Google Console](https://console.cloud.google.com).
