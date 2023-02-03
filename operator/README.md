# AIS Kubernetes Operator

## Overview
AIStore is designed to run natively on Kubernetes.
This folder contains **AIS Operator** that provides for bootstrapping, deployment, scaling up (and down), gracefully shutting down, upgrading, and managing resources of AIS clusters on Kubernetes. Technically, the project extends native Kubernetes API to automate management of all aspects of the AIStore lifecycle.

> **WARNING:** AIS K8S Operator (or, simply, AIS Operator) is currently undergoing active development - non-backward compatible changes are to be expected at any moment.

### Walkthrough
If you'd like to get started quickly, you can find a [walkthrough here](../docs/walkthrough.md), taking you through a complete AIStore deployment using the operator.

## Deploying AIS Cluster
### Prerequisites
* K8s cluster
* `kubectl`

To deploy AIS operator on an existing K8s cluster, execute the following commands:

AIS operator employs [admission webhooks](https://kubernetes.io/docs/reference/access-authn-authz/extensible-admission-controllers/) to enforce the validity of the managed AIS cluster.
AIS operator runs a webhook server with `tls` enabled, responsible for validating each AIS cluster resource being created or updated.
[Operator-SDK](https://sdk.operatorframework.io/) recommends using [cert-manager](https://github.com/jetstack/cert-manager) for provisioning the certificates required by the webhook server, however, any solution which can provide certificates to the AIS operator pod at location `/tmp/k8s-webhook-server/serving-certs/tls.(crt/key)`, should work.

For quick deployment, the `deploy` command provides an option to deploy a basic version of `cert-manager`. However, for more advanced deployments it's recommended to follow [cert-manager documentation](https://cert-manager.io/docs/installation/kubernetes/).

```console
# Deploy AIS Operator
$ IMG=aistore/ais-operator:latest make deploy
would you like to deploy cert-manager? [y/n]
...
# Ensure the operator is ready
$ kubectl get pods -n ais-operator-system
NAME                                               READY   STATUS    RESTARTS   AGE
ais-operator-controller-manager-64c8c86f7b-8g8pj   2/2     Running   0          18s

# Deploy sample AIS Cluster
$ kubectl apply -f config/samples/ais_v1beta1_aistore.yaml -n ais-operator-system
$ kubectl get pods -n ais-operator-system
NAME                                                  READY   STATUS    RESTARTS   AGE
ais-operator-v2-controller-manager-64c8c86f7b-2t6jg   2/2     Running   0          5m23s
aistore-sample-proxy-0                                1/1     Running   0          2m8s
aistore-sample-target-0                               1/1     Running   0          2m21s
```

### Enabling external access

This section discusses AIStore accessibility by external clients - the clients **outside the Kubernetes cluster**.
To achieve that, AIS Operator utilizes K8s `LoadBalancer` service.

Generally, external access relies on the K8s capability to assign an external IP (or hostname) to a `LoadBalancer` services.
Enabling external access is as easy as setting `enableExternalLB` to `true` while `applying` the AIStore cluster resource.

For instance, you could update `config/samples/ais_v1beta1_aistore.yaml` as follows:

```yaml
# config/samples/ais_v1beta1_sample.yaml
apiVersion: ais.nvidia.com/v1beta1
kind: AIStore
metadata:
  name: aistore-sample
spec:
  ...
  enableExternalLB: true
  # enableExternalLB: false
```

> NOTE: Currently, external access can be enabled only for new AIS clusters. Updating the `enablingExternalLB` spec for an existing cluster is not yet supported.

Another important consideration is - the number of external IPs.
To deploy an AIS cluster of N storage nodes, the K8s cluster will have to assign external IPs to (N + 1) `LoadBalancer` services: one for each storage target plus one more for all the AIS proxies (aka AIS gateways) in that same cluster.

Failing that requirement will lead to a failure to deploy AIStore.

External access can be tested locally on `minikube` using the following command:

```console
$ minikube tunnel
```

For more information and details on *minikube tunneling*, please see [this link](https://minikube.sigs.K8s.io/docs/commands/tunnel/).

### Deploying cluster with shared or no disks

For development/testing K8s setup where the `mountpaths` attached to the storage targets pods are not block devices, i.e. have no disks, or share the disk, will result in the target pods to fail with `has no disks` or `filesystem sharing is not allowed` error.
To deploy AIStore cluster on such K8s environments is possible by setting the `allowSharedNoDisks` property to `true`, as follows:

```yaml
# config/samples/ais_v1beta1_sample.yaml
apiVersion: ais.nvidia.com/v1beta1
kind: AIStore
metadata:
  name: aistore-sample
spec:
  size: 4 # > number of K8s nodes
  allowSharedNoDisks: true
...
```

> **WARNING:** It is NOT recommended to set the `allowSharedNoDisks` property to `true` for production deployments.


### Locally testing multi-node AIS cluster

By default, AIS operator restricts having more than one AIS target per K8s node. In other words, if AIS custom resource spec has a `size` greater than the number of K8s nodes, additional target pods will remain pending until we add a new K8s node.

However, this constraint can be relaxed for local testing using the `disablePodAntiAffinity` property as follows:

```yaml
# config/samples/ais_v1beta1_sample.yaml
apiVersion: ais.nvidia.com/v1beta1
kind: AIStore
metadata:
  name: aistore-sample
spec:
  size: 4 # > number of K8s nodes
  disablePodAntiAffinity: true
...
```

## Development

AIS Operator leverages [operator-sdk](https://github.com/operator-framework/operator-sdk), which provides high-level APIs, scaffolding, and code generation utilities, making the operator development easy.

[operator/api/v1beta1](operator/api/v1beta1), contains `go` definitions for Custom Resource Definitions (CRDs) and Webhooks used by the operator.
Any modifications to these type definitions requires updating of the auto-generated code ([operator/api/v1beta1/zz_generated.deepcopy.go](operator/api/v1beta1/zz_generated.deepcopy.go)) and the YAML manifests used for deploying operator related K8s resources to the cluster.
We use the following commands to achieve this:

```console
$ # updating the auto-generated code
$ make generate
$ # updating the YAML manifests under config/
$ make manifests
```

For building and pushing the operator docker images, use the following commands:

```console
$ # building the docker image
$ IMG=<REPOSITORY>/<IMAGE_TAG> make docker-build
$ # pushing the docker image
$ IMG=<REPOSITORY>/<IMAGE_TAG> make docker-push
```

To deploy and test local changes using `minikube`, we recommend enabling and using docker registry with minikube, by using the following commands:

```console
$ # enable registry
$ minikube addons enable registry

$ # removing minikube's registry-fwd container if preset
$ docker kill registry-fwd || true

$ # map localhost:5000 to the registry of minikube
$ docker run --name registry-fwd --rm -d -it --network=host alpine ash -c "apk add socat && socat TCP-LISTEN:5000,reuseaddr,fork TCP:$(minikube ip):5000"


$ # build, push and deploy operator
$ IMG=localhost:5000/opr-test:1 make docker-build docker-push deploy
```

## Testing

Testing AIS operator requires having a running K8s cluster. You could run the tests using the following command:

```console
$ make test
```
Some tests require the K8s cluster to allocate external IP addresses. For a `minikube` based deployment, you could use `tunnel` as described [here](#enabling-external-access)

To use a custom K8s storage-class for tests set the environment variable `TEST_STORAGECLASS` as follows:
```console
$ TEST_STORAGECLASS="standard" make test
```
