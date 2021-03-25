# AIStore Cloud Deployment with Terraform

The AIStore project aims to be easy to deploy on the most common cloud platforms.
Terraform is the project's tool of choice to easily automate deployments on a wide range of substrates.

This directory contains Terraform definitions and scripts that enable deploying AIStore clusters on Kubernetes in the cloud.
Terraform is used to deploy Kubernetes on a specified cloud provider, then `kubectl` and `helm` are used to deploy AIStore on Kubernetes.
The main script (`deploy.sh`) will walk you through the required steps to set up the AIStore cluster.

<img src="docs/images/ais-k8s-deploy.gif" alt="Deploy K8s cluster with AIStore" width="80%">

If you have an existing Kubernetes cluster, regardless of a cluster provider, you can deploy AIStore
to that running cluster [using arguments](#supported-arguments) on the deploy script.

Pre-requisites:

* A cloud account (GCP is given as an example).
* Terraform, kubectl, and helm client [commands line tools](#appendix-client-workstation-prep).

## Supported Cloud Providers

| Provider | ID | Required Commands |
| -------- | --- | ----------------- |
| Google (GCP, GKE) | `gcp` | `gcloud` |

### Google

In `gcp/main.tf` file, you can find a couple of variables that can be adjusted to your preferences:
* `zone` - zone in which the cluster will be deployed (for now, it's only possible to deploy a cluster on a single zone; using [regional cluster](https://cloud.google.com/kubernetes-engine/docs/concepts/types-of-clusters#regional_clusters) is not yet supported).
* `machine_type` - machine type which will be used as GKE nodes (see [full list](https://cloud.google.com/compute/docs/machine-types)).
* `machine_preemptible` - determines if the machine is preemptible (more info [here](https://cloud.google.com/compute/docs/instances/preemptible)).

## Deploy

To deploy a new Kubernetes + AIStore cluster, run the `./deploy.sh all` script and follow the instructions.
When the script successfully finishes, the AIStore cluster should be accessible and ready to use.

```console
$ ./deploy.sh all
```

Alternatively, deploy AIStore to an existing Kubernetes cluster as follows:

```console
$ ./deploy.sh ais
```

To deploy AIStore with cloud providers, you need to provide valid credentials using the provider flag. 
For instance, to deploy AIStore with AWS provider, use the `--aws` flag to provide credentials directory as follows: 

```console
$ ./deploy.sh ais --aws="/home/ubuntu/.aws"
```

Additionally, you can deploy only Kubernetes cluster, which will be ready to run AIS cluster.

```console
$ ./deploy.sh k8s
```

### Supported Arguments

`./deploy.sh DEPLOY_TYPE [--flag=value ...]`

There are 3 `DEPLOY_TYPE`s:
* `all` - start nodes on the specified provider, start K8s cluster and deploy AIStore on K8s nodes.
* `ais` - only deploy AIStore on K8s nodes, assumes that K8s cluster is already deployed.
* `k8s` - only deploy K8s cluster, without deploying AIS cluster.
* `dashboard` - start [K8s dashboard](https://kubernetes.io/docs/tasks/access-application-cluster/web-ui-dashboard) connected to started K8s cluster.

| Flag | Description | Default value |
| ---- | ----------- | ------------- |
| `--cloud` | Cloud provider to be used (`aws`, `azure` or `gcp`). | - |
| `--node-cnt` | Number of instances/nodes to be started. | - |
| `--disk-cnt` | Number of disks per instance/node. | - |
| `--cluster-name` | Name of the Kubernetes cluster. | - |
| `--wait` | Maximum timeout to wait for all the Pods to be ready. | `false` |
| `--aisnode-image` | The image name of `aisnode` container. | `aistore/aisnode:3.3.1` |
| `--admin-image` | The image name of `admin` container. | `aistore/admin:3.3` |
| `--dataplane` | Network dataplane to be used (`kube-proxy` or `cilium`) | `kube-proxy` |
| `--expose-external` | Will expose AIStore cluster externally by assigning an external IP. |
| `--aws` | Path to AWS credentials directory. | - |
| `--help` | Show help message. | - |

### Admin container

After full deployment you should be able to list all K8s Pods:
```console
$ ./deploy.sh all --cloud=gcp --node-cnt=2 --disk-cnt=2 --wait=5m
...
$ kubectl get pods
NAME                   READY   STATUS    RESTARTS   AGE
demo-ais-admin-99p8r   1/1     Running   0          31m
demo-ais-proxy-5vqb8   1/1     Running   0          31m
demo-ais-proxy-g7jf7   1/1     Running   0          31m
demo-ais-target-0      1/1     Running   0          31m
demo-ais-target-1      1/1     Running   0          29m
```

As you can see, there is one particular Pod called `demo-ais-admin-*`.
It contains useful binaries:
 * `ais` (more [here](github.com/NVIDIA/aistore/cmd/cli/README.md)),
 * `aisloader` (more [here](github.com/NVIDIA/aistore/bench/aisloader/README.md)),
 * `xmeta` (more [here](github.com/NVIDIA/aistore/cmd/xmeta/README.md)).

Thanks to them, you can access and stress-load the cluster.

After logging into the container, the commands are already configured to point to the deployed cluster:
```console
$ kubectl exec -it demo-ais-admin-99p8r -- /bin/bash
root@demo-ais-admin-99p8r:/#
root@demo-ais-admin-99p8r:/# ais show cluster
PROXY		 MEM USED %	 MEM AVAIL	 CPU USED %	 UPTIME	 STATUS
rOFMYYks	 0.79		 3.60GiB	 0.00		 49m	 healthy
zloxzvzK[P]	 0.82		 3.60GiB	 0.00		 50m	 healthy

TARGET		 MEM USED %	 MEM AVAIL	 CAP USED %	 CAP AVAIL	 CPU USED %	 REBALANCE		 UPTIME	 STATUS
BEtMbslT	 0.83		 3.60GiB	 0		 99.789GiB	 0.00		 finished, 0 moved (0B)	 49m	 healthy
MbXeFcFw	 0.84		 3.60GiB	 0		 99.789GiB	 0.00		 finished, 0 moved (0B)	 48m	 healthy


Summary:
 Proxies:	2 (0 - unelectable)
 Targets:	2
 Primary Proxy:	zloxzvzK
 Smap Version:	8
```

### Network Dataplane

The deployment scripts provide an option to replace the default [kube-proxy](https://kubernetes.io/docs/reference/command-line-tools-reference/kube-proxy/) networking dataplane with [Cilium](https://cilium.io/), an open-source project that provides a highly scalable [eBPF](https://ebpf.io/) based K8S CNI.

Replacing `kube-proxy` with Cilium enables us to leverage the Direct Server Return (DSR) feature, which allows the AIStore targets to reply directly to the external client avoiding any additional network hops, hence reducing network latency.

To deploy AIS with Cilium enabled, we use the `--dataplane` argument as follows:
```console
$ ./deploy.sh all --cloud=gcp --node-cnt=2 --disk-cnt=2 --dataplane=cilium
```

Above command deploys Cilium components and replaces `kube-proxy`.
To verify the setup, execute the following commands:
```console
$ kubectl get pods -n cilium
NAME                               READY   STATUS             RESTARTS   AGE
cilium-27n89                       1/1     Running            0          56m
cilium-48xwz                       1/1     Running            0          56m
cilium-node-init-npz48             1/1     Running            0          56m
cilium-node-init-twqvl             1/1     Running            0          56m
cilium-operator-86cc7c989b-8qsz8   1/1     Running            0          56m
cilium-operator-86cc7c989b-ql6rg   1/1     Running            0          56m

$ # To verify if AIS services use cilium
$ kubectl exec -it  -n cilium cilium-48xwz -- cilium service list 
ID   Frontend                 Service Type   Backend                   
...
9    10.3.245.68:51081        ClusterIP      1 => 10.0.1.3:51081       
                                             2 => 10.0.2.118:51081     
10   0.0.0.0:31809            NodePort       1 => 10.0.1.3:51081       
                                             2 => 10.0.2.118:51081     
...
```

### AIStore external access

WARNING: Enabling external access will provide unrestricted access to anyone with the external IP. Therefore this option is only intended for demo/testing purposes.
For production use, it's advised to enable HTTPS, Auth, and other security features that are not included in this script.

To allow external clients to access the AIStore deployment, set the `--expose-external` flag while deploying AIS as follows:

```console
$ ./deploy.sh ais --expose-external
...
$ kubectl get services
NAME                      TYPE           CLUSTER-IP     EXTERNAL-IP       PORT(S)                         AGE
demo-ais-gw               LoadBalancer   10.3.248.251   168.121.56.107   51080:32643/TCP                 40m
demo-ais-proxy            ClusterIP      None           <none>            51080/TCP                       40m
demo-ais-proxy-external   LoadBalancer   10.3.248.56    102.69.81.190     51080:30691/TCP                 40m
demo-ais-target           ClusterIP      None           <none>            51081/TCP,51082/TCP,51083/TCP   40m
demo-ais-target-0         LoadBalancer   10.3.250.48    159.163.144.57    51081:32582/TCP                 40m
demo-ais-target-1         LoadBalancer   10.3.252.56    26.127.50.217     51081:30825/TCP                 40m
```
As you can see, for an AIStore deployment with two targets, an external IP address is assigned to each target using a K8S LoadBalancer service. This setup allows external clients to connect to the targets to access data directly.

You may test the deployment as follows:
```console
$ # Set AIS_ENDPOINT to point to external IP of `demo-ais-proxy-external` service
$ export AIS_ENDPOINT="http://102.69.81.190:51080"
$ ais show cluster smap
DAEMON ID        TYPE    PUBLIC URL
AJnmssQh         proxy   http://10.0.1.198:51080
YTBHKlCX[P]      proxy   http://10.0.2.22:51080

DAEMON ID        TYPE    PUBLIC URL
JDvxawSR         target  http://159.163.144.57:51081
QGXpMjRa         target  http://26.127.50.217:51081

Non-Electable:

Primary Proxy: YTBHKlCX
Proxies: 2       Targets: 2      Smap Version: 7

$ # Put and get objects
$ mkdir tmp && for i in {0..100}; do echo "HELLO" > tmp/${i}; done
$ ais create bucket test
$ ais put tmp ais://test
Files to upload:
EXTENSION        COUNT   SIZE
                 101     606B
TOTAL           101     606B
Proceed uploading to bucket "ais://test"? [Y/N]: y
101 objects put into "ais://test" bucket
$ ais get test/2 -
HELLO
"2" has the size 6B (6 B)
```

## Destroy

To remove and clean up the cluster, we have created `destroy.sh` script.
Similarly to the deploy script, it will walk you through the required steps to clean up the cluster.

### Supported arguments

`./destroy.sh DESTROY_TYPE [--flag=value ...]`

There are 2 `DESTROY_TYPE`s:
* `all` - stop AIStore Pods, and destroy started K8s nodes.
* `ais` - only stop AIStore Pods so that the cluster can be redeployed.

| Flag | Description |
| ---- | ----------- |
| `--preserve-disks` | Do not remove persistent volumes - data on targets. It will be available on the next deployment. Not supported with `all`. |
| `--help` | Show help message. |

## Troubleshooting

### Google

#### Insufficient permissions

> googleapi: Error 403: Required '...' permission(s) for '...', forbidden

This may happen when an account does not have enough permissions to create or access a specific GCP resource.

What you can do:
* Try to run:
    ```console
    $ gcloud auth application-default login
    ```
* Make sure `GOOGLE_APPLICATION_CREDENTIALS` is not set to credentials for other project/account.
* Make sure you have the right permissions set for your account in [Google Console](https://console.cloud.google.com).

#### Insufficient quota

> googleapi: Error 403: Insufficient regional quota to satisfy request ...

This may happen if an account's or project's quota is exceeded.
Running the cluster on a free account can easily exceed available resources (for a free account, we recommend 2-4 nodes and 1-3 disks per node).

What you can do:
* Try to run the cluster with fewer nodes (something between 2-4).
* Try to run the cluster with fewer disks per node (something between 1-3).
* Try to increase the quota on [GCP Console](https://console.cloud.google.com/iam-admin/quotas).

## Appendix: Client Workstation Prep

### For Ubuntu 20.04 and later:

#### Install Google Cloud Command Line Tool

```console
$ sudo snap install google-cloud-sdk --classic
$ gcloud init
```

*Reference:*
* `gcloud`
  * https://cloud.google.com/sdk/docs/downloads-snap

#### Install Local Command Line Tools for Kubernetes, Docker, et al.

```console
$ sudo snap install docker
$ sudo snap install kubectl --classic
$ sudo snap install helm --classic
```

*References:*
* `docker`
  * https://docs.docker.com/get-docker/
  * https://snapcraft.io/docker
* `kubectl`
  * https://kubernetes.io/docs/tasks/tools/install-kubectl/
  * https://snapcraft.io/kubectl
* `helm`
  * https://helm.sh/docs/intro/install/
  * https://snapcraft.io/helm

#### Install Local Terraform Command Line Tool

```console
$ curl -fsSL https://apt.releases.hashicorp.com/gpg | sudo apt-key add -
$ sudo apt-add-repository "deb [arch=amd64] https://apt.releases.hashicorp.com $(lsb_release -cs) main"
$ sudo apt update
$ sudo apt install terraform
```

*Reference:*
* `terraform`
  * https://learn.hashicorp.com/tutorials/terraform/install-cli

### For Mac OS:

#### Install Google Cloud Command Line Tool

```console
$ brew cask install google-cloud-sdk
$ gcloud init
```

*Reference:*
* `gcloud`
  * https://cloud.google.com/sdk/docs/install#mac

#### Install Local Command Line Tools for Kubernetes, Docker, et al.

```console
$ # For Docker we recommend using 'Docker for Mac' (https://docs.docker.com/docker-for-mac/install/)
$ brew install kubectl
$ brew install helm
```

*References:*
* `docker`
  * https://docs.docker.com/docker-for-mac/install/
  * https://docs.docker.com/get-docker/
* `kubectl`
  * https://kubernetes.io/docs/tasks/tools/install-kubectl/
* `helm`
  * https://helm.sh/docs/intro/install/
  * https://formulae.brew.sh/formula/helm

#### Install Local Terraform Command Line Tool

```console
$ brew install terraform
```

*Reference:*
* `terraform`
  * https://learn.hashicorp.com/tutorials/terraform/install-cli
  * https://formulae.brew.sh/formula/terraform
