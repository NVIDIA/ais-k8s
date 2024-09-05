# AIS Kubernetes Operator

## Overview
AIStore is designed to run natively on Kubernetes.
This folder contains **AIS Operator** that provides for bootstrapping, deployment, scaling up (and down), gracefully shutting down, upgrading, and managing resources of AIS clusters on Kubernetes. Technically, the project extends native Kubernetes API to automate management of all aspects of the AIStore lifecycle.

> **WARNING:** AIS K8S Operator (or, simply, AIS Operator) is currently undergoing active development - non-backward compatible changes are to be expected at any moment.

### Production Deployments
If you want to deploy an AIStore Cluster in production setting we recommend deploying AIStore using our [ansible playbooks](../playbooks). We have provided detailed, step-by-step instructions for deploying AIStore on Kubernetes (K8s) in this [guide](../docs/README.md).

## Deploying AIS Cluster
### Prerequisites
* K8s cluster
* `kubectl`

To deploy AIS operator on an existing K8s cluster, execute the following commands:

AIS operator employs [admission webhooks](https://kubernetes.io/docs/reference/access-authn-authz/extensible-admission-controllers/) to enforce the validity of the managed AIS cluster.
AIS operator runs a webhook server with `tls` enabled, responsible for validating each AIS cluster resource being created or updated.
[Operator-SDK](https://sdk.operatorframework.io/) recommends using [cert-manager](https://github.com/jetstack/cert-manager) for provisioning the certificates required by the webhook server, however, any solution which can provide certificates to the AIS operator pod at location `/tmp/k8s-webhook-server/serving-certs/tls.(crt/key)`, should work.

For quick deployment, the `deploy` command provides an option to deploy a basic version of `cert-manager`. However, for more advanced deployments it's recommended to follow [cert-manager documentation](https://cert-manager.io/docs/installation/kubernetes/).


### Deploy AIS Operator
```console
$ IMG=aistorage/ais-operator:latest make deploy
```

### Ensure the operator is ready
```console
$ kubectl get pods -n ais-operator-system
NAME                                               READY   STATUS    RESTARTS   AGE
ais-operator-controller-manager-64c8c86f7b-8g8pj   2/2     Running   0          18s
```

### Deploy sample AIS Cluster
**Note: If you are testing on minikube with multiple mounts, each mount defined in the AIS spec must have the same label**
```console
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

In a development/testing K8s setup, the `mountpaths` attached to storage target pods may either be block devices (no disks) or share a disk. 
This will result in target pod errors such as `has no disks` or `filesystem sharing is not allowed`.
To deploy AIStore cluster on such K8s environments, set a shared label for each mountpath as follows:

```yaml
# config/samples/ais_v1beta1_sample.yaml
apiVersion: ais.nvidia.com/v1beta1
kind: AIStore
metadata:
  name: aistore-sample
spec:
  targetSpec:
    mounts:
      - path: "/ais1"
        size: 10Gi
        label: "disk1"
      - path: "/ais2"
        size: 10Gi
        label: "disk1"
...
```

The above spec will tell AIS to allow both mounts to share a single disk as long as the `label` is the same. If the `label` does not exist as an actual disk, the target pod will accept it and run in diskless mode without disk statistics. 

> **WARNING:** `allowSharedNoDisks` is deprecated. If you are using a cluster with allowSharedNoDisks, first update the operator to the latest version for compatibility with the latest AIS versions. `allowSharedNoDisks` will be removed in a future update. 

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

### Config Backend Provider for GCP & AWS

AIS operator supports GCP and AWS as the config backend provider. To enable the config backend provider, you need to create a secret with the corresponding credential file.

```yaml
kubectl create secret -n ais-operator-system generic aws-creds \
  --from-file=config=$HOME/.aws/config \
  --from-file=credentials=$HOME/.aws/credentials

kubectl create secret -n ais-operator-system generic gcp-creds \
  --from-file=gcp.json=<path-to-gcp-credential-file>.json
```

```yaml
# config/samples/ais_v1beta1_sample.yaml
apiVersion: ais.nvidia.com/v1beta1
kind: AIStore
metadata:
  name: aistore-sample
spec:
  gcpSecretName: "gcp-creds" # corresponding secret name just created for GCP credential
  awsSecretName: "aws-creds" # corresponding secret name just created for AWS credential
...
```

### Enabling HTTPS for AIStore Deployment in Kubernetes

While the examples above demonstrate running web servers that accept plain HTTP requests, you may want to enhance the security of your AIStore deployment by enabling HTTPS in a Kubernetes environment.

**Important:** Before proceeding, please ensure that you have `cert-manager` installed.

This specification defines a ClusterIssuer responsible for certificate issuance. It creates a Certificate, which is securely stored as a Secret within the same namespace as the operator.

```bash
kubectl apply -f config/samples/ais_v1beta1_aistore_tls_selfsigned.yaml
```

With `cert-manager csi-driver` installed, you can get signed certificates directly from your Issuer. The attached sample configuration contains RBAC and Issuer definition for use with Vault.

```bash
kubectl apply -f  config/samples/ais_v1beta1_aistore_tls_certmanager_csi.yaml
```

**Testing Considerations:**

- For tests utilizing the AIStore Command Line Interface (CLI), configure the CLI to bypass certificate verification by applying the setting: execute `$ ais config cli set cluster.skip_verify_crt true`. This adjustment facilitates unverified connections to the AIStore cluster.

- When using `curl` to interact with your AIStore cluster over HTTPS, use the `-k` flag to skip certificate validation. For example:

```bash
curl -k https://your-ais-cluster-url
```

- If you prefer not to skip certificate validation, you can export the self-signed certificate for use with `curl`. Here's how to export the certificate:

```bash
kubectl get secret tls-certs -n ais-operator-system -o jsonpath='{.data.tls\.crt}' | base64 --decode > tls.crt
```

You can now use the exported `tls.crt` as a parameter when using `curl`, like this:

```bash
curl --cacert tls.crt https://your-ais-cluster-url
```

By following these steps, you can deploy AIStore in a Kubernetes environment with HTTPS support, leveraging a self-signed certificate provided by cert-manager.

## Development

AIS Operator leverages [operator-sdk](https://github.com/operator-framework/operator-sdk), which provides high-level APIs, scaffolding, and code generation utilities, making the operator development easy.

[operator/api/v1beta1](operator/api/v1beta1), contains `go` definitions for Custom Resource Definitions (CRDs) and Webhooks used by the operator.
Any modifications to these type definitions requires updating of the auto-generated code ([operator/api/v1beta1/zz_generated.deepcopy.go](operator/api/v1beta1/zz_generated.deepcopy.go)) and the YAML manifests used for deploying operator related K8s resources to the cluster.
We use the following commands to achieve this:

```console
$ # Updating the auto-generated code.
$ make generate
$
$ # Updating the YAML manifests under `config/`.
$ make manifests
```

For building and pushing the operator Docker images, use the following commands:

```console
$ # Building the Docker image.
$ IMG=<REPOSITORY>/<IMAGE_TAG> make docker-build
$
$ # Pushing the Docker image.
$ IMG=<REPOSITORY>/<IMAGE_TAG> make docker-push
```

## Testing

Testing the AIS operator is categorized into two groups: tests that require a Kubernetes cluster and those that do not.

### Unit tests

You can run unit tests without an existing cluster by executing:
```console
$ make test
```

You can also run unit tests with existing cluster with:
```console
$ export USE_EXISTING_CLUSTER=true 
$ make test
```

### End-to-End (E2E) tests

E2E tests require an existing Kubernetes cluster.
To run them, execute:
```console
$ make test-e2e-short
$
$ # To use a custom Kubernetes `StorageClass` for tests set the environment 
$ # variable `TEST_STORAGECLASS` as follows:
$ TEST_STORAGECLASS="standard" make test-e2e-short
```

### `kind` cluster

You can create a Kubernetes cluster using the [`kind` tool](https://kind.sigs.k8s.io/).
To make `kind` work you need to have `docker` or `podman` installed.

To bootstrap `kind` cluster run:
```console
$ make kind-setup
$
$ # You can also specify which Kubernetes version should be used to bootstrap the cluster.
$ # For example:
$ KIND_K8S_VERSION="v1.30.2" make kind-setup 
```

To tear down the local cluster after testing, run:
```bash
make kind-teardown
```

#### Running tests

After that you can run tests:
```console
$ export USE_EXISTING_CLUSTER=true 
$ make test
```
**Note:** Running E2E tests with a `kind` cluster might be possible, but we cannot guarantee full compatibility at this time.

#### Testing local changes

You can also build your local changes and test them using `kind` cluster:
```console
$ export IMG=ais-operator:testing
$ make docker-build
$ kind load docker-image --name ais-operator-test "${IMG}"
$
$ kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.10.0/cert-manager.yaml
$ wait 20
$
$ # Make sure that in `config/manager/manager.yaml` you set `imagePullPolicy: Never`.
$ # This makes sures that `kind` cluster won't be trying to download this image from outside. 
$ make deploy
```

After that you can deploy your `AIStore` CRD and test if everything works properly.

### `minikube` cluster

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

Some tests require the K8s cluster to allocate external IP addresses.
For a `minikube` based deployment, you could use `tunnel` as described [here](#enabling-external-access)
