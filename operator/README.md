# AIS Kubernetes Operator

## Overview
AIStore is designed to run natively on Kubernetes.
This folder contains **AIS K8S Operator** that provides for deployment, bootstrapping, scaling up (and down), upgrading, and managing resources of the AIS clusters on Kubernetes.

> **WARNING:** AIS K8S Operator (or, simply, AIS Operator) is currently undergoing active development - non-backward compatible changes are to be expected at any moment.

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
$ kubectl apply -f config/samples/ais_v1alpha1_aistore.yaml -n ais-operator-system
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

For instance, you could update `config/samples/ais_v1alpha1_aistore.yaml` as follows:

```yaml
# config/samples/ais_v1alpha1_sample.yaml
apiVersion: ais.nvidia.com/v1alpha1
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


### Locally testing multi-node AIS cluster

By default, AIS operator restricts having more than one AIS target per K8s node. In other words, if AIS custom resource spec has a `size` greater than the number of K8s nodes, additional target pods will remain pending until we add a new K8s node.

However, this constraint can be relaxed for local testing using the `disablePodAntiAffinity` property as follows:

```yaml
# config/samples/ais_v1alpha1_sample.yaml
apiVersion: ais.nvidia.com/v1alpha1
kind: AIStore
metadata:
  name: aistore-sample
spec:
  size: 4 # > number of K8s nodes
  disablePodAntiAffinity: true
...
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
