# AIS Kubernetes Operator

## Overview
AIStore is designed to run natively on Kubernetes.
This folder contains the **AIS Operator** for managing the resources and lifecycle of AIS clusters on Kubernetes.
The project extends the native Kubernetes API to deploy, scale, upgrade, decommission, and otherwise automate management of all aspects of the AIStore lifecycle.

> **WARNING:** The AIS K8s Operator is currently undergoing active development. Please see the [compatibility docs](../docs/COMPATIBILITY.md) for info on upgrades and deprecations.

### Production Deployments

See our guide for deploying [AIStore on Kubernetes](../docs/README.md).

## Deploying AIS
### Prerequisites

To deploy the operator, only a K8s cluster, `kubectl`, and a certificate provider (see below) are required. 
Check out our [prerequisites doc](../docs/prerequisites.md) for production deployment requirements. 

### Operator Deployment Options

AIS operator employs [admission webhooks](https://kubernetes.io/docs/reference/access-authn-authz/extensible-admission-controllers/) to enforce the validity of the managed AIS cluster.
AIS operator runs a webhook server with `tls` enabled, responsible for validating each AIS cluster resource being created or updated.
[Operator-SDK](https://sdk.operatorframework.io/) recommends using [cert-manager](https://github.com/jetstack/cert-manager) for provisioning the certificates required by the webhook server.
However, any solution which can provide certificates to the AIS operator pod should work.
The operator loads from the `webhook-cert-path` arg which defaults to `/tmp/k8s-webhook-server/serving-certs/`.

For quick deployment, the `deploy` command provides an option to deploy a basic version of `cert-manager`.
However, for more advanced deployments it's recommended to follow [cert-manager documentation](https://cert-manager.io/docs/installation/kubernetes/).

### Configure Operator TLS and mTLS

The operator communicates with the deployed AIS clusters over the AIS API.
Today, by default, if the AIS cluster is using HTTPS the operator will not verify the certificate. 
Certificate verification can be enabled by setting `OPERATOR_SKIP_VERIFY_CRT` to `False` in the operator deployment yaml (kustomize generated from [config/default/manager_env_patch.yaml](config/default/manager_env_patch.yaml))

If your AIS cluster uses an untrusted CA, you can configure trust for verification by creating a configMap `ais-operator-ais-ca` in the operator namespace before starting the operator pod.
This will automatically mount to `/etc/ais/ca` and add any `.crt` or `.pem` files as trusted CA certificates.

**Note for trust-manager/cert-manager clusters**: If you're using cert-manager with trust-manager and want to use the automatically distributed CA bundle (e.g., `lepton-ca-bundle`), you can create a simple kustomize patch to override the ConfigMap name. For example:

```yaml
# config/overlays/custom/manager_ca_patch.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: controller-manager
  namespace: system
spec:
  template:
    spec:
      volumes:
        - name: ais-ca  # or authn-ca for AuthN
          configMap:
            name: lepton-ca-bundle  # Override with your trust-manager bundle name
            optional: true
```

#### AuthN TLS Configuration

The operator also communicates with AuthN services for authentication. To configure TLS for AuthN connections:

1. **Create a ConfigMap** with your AuthN CA certificate:
   ```bash
   kubectl create configmap ais-operator-authn-ca \
     --from-file=ca.crt=/path/to/authn-ca.pem \
     -n ais-operator-system
   ```

   **Note for trust-manager/cert-manager clusters**: If you're using cert-manager with trust-manager and want to use the automatically distributed CA bundle, create a kustomize patch to override the ConfigMap name (see example in the AIS cluster CA section above).

2. **Restart the operator** to pick up the ConfigMap:
   ```bash
   kubectl rollout restart deployment/ais-operator-controller-manager \
     -n ais-operator-system
   ```

3. **Configure the AIStore CR** to use the mounted certificate:
   ```yaml
   spec:
     auth:
       serviceURL: https://ais-authn.ais:52001
       tls:
         caCertPath: /etc/ssl/certs/authn-ca/ca.crt  # Mounted from ConfigMap
   ```

The ConfigMap is **optional** - the operator pod starts successfully even if the ConfigMap doesn't exist (using `optional: true` in the volume definition). If you create the ConfigMap after the operator is running, restart the operator deployment to pick it up.

**Note**: If you update an existing ConfigMap, the changes automatically propagate to running pods within ~60 seconds (kubelet sync), and the operator will use the new certificates after the cache TTL expires (default 6 hours).

**Performance Note**: TLS configurations are cached for 6 hours by default to avoid repeated disk I/O. This can be adjusted via environment variable:

```yaml
env:
  - name: OPERATOR_AUTH_TLS_CACHE_TTL
    value: "1h"  # Adjust for frequent certificate rotations
```

#### Mutual TLS / Client Auth

To enable mutual TLS (mTLS) between the operator and an AIS cluster, first create a certificate with `usage: client auth` defined (see [cert-manager docs](https://cert-manager.io/docs/usage/certificate/)).

You can mount this into the pod with a tool such as the [Vault agent](https://developer.hashicorp.com/vault/docs/agent-and-proxy/agent), or you can create a secret `operator-tls` in the operator namespace.
This secret will be mounted by default at `/etc/operator/tls`. 
The operator will use the client certificate at this location when communicating with AIS clusters.

To configure the location of the operator's client cert, use the `ais-client-cert-path` when running the manager. 

If the `ais-client-cert-per-cluster` arg is provided, the operator will load the value from `ais-client-cert-path` and append the values from each cluster's namespace and name when loading certificates.
For example, `/etc/operator/tls/aisNamespace/aisCluster`.

See the linked [certificates diagram](../docs/diagrams/certificates.jpg) for a visualization of the TLS options.

### Deploy AIS Operator

First, install the AIS CRD
```console
make install
```
Then run the deployment. This will apply the [default kustomization](./config/default/kustomization.yaml) configuration. 
```console
$ IMG=aistorage/ais-operator:latest make deploy
```

### Ensure the operator is ready
```console
$ kubectl get pods -n ais-operator-system
NAME                                               READY   STATUS    RESTARTS   AGE
ais-operator-controller-manager-64c8c86f7b-8g8pj   1/1     Running   0          18s
```

### Deploy sample AIS Cluster
**Note: If you are testing on minikube with multiple mounts, each mount defined in the AIS spec must have the same label**
```console
$ kubectl create namespace ais
$ kubectl apply -f config/samples/ais_v1beta1_aistore.yaml -n ais
$ kubectl get pods -n ais
NAME                                                  READY   STATUS    RESTARTS   AGE
aistore-sample-proxy-0                                1/1     Running   0          2m8s
aistore-sample-target-0                               1/1     Running   0          2m21s
```

### Enabling external access

This section discusses AIStore accessibility by external clients - the clients **outside the Kubernetes cluster**.

By default, each AIS pod will deploy with a `HostPort` configuration, allowing any client with access to the host to communicate to the pod directly over the specified port. 

The AIStore custom resource also contains the `enableExternalLB` setting, which will instruct the operator to create K8s `LoadBalancer` services for the pods.
External access relies on the K8s capability to assign an external IP (or hostname) to these `LoadBalancer` services.

**Setting up external IPs**
- **Bare-Metal On-Premises Deployments**: For these setups, we recommend using [MetalLB](https://metallb.universe.tf/), a popular solution for on-premises Kubernetes environments.
- **Cloud-Based Deployments**: If your AIStore is running in a cloud environment, you can utilize standard HTTP load balancer services provided by the cloud provider.

**enableExternalLB example**
Update your AIS spec as follows:

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
To deploy AIStore cluster in such K8s environments, set a shared label for each mountpath as follows:

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

### Deploying cluster with distributed tracing enabled

AIS Operator supports deploying AIStore with distributed tracing enabled. To get started, below instructions demonstrate how to enable distributed tracing and export traces to [Lightstep](https://docs.lightstep.com/).

#### Prerequisites
- **Create a Lightstep Freemium Account**  
  Sign up for a Lightstep account if you haven't already: [Lightstep Sign-Up](https://info.servicenow.com/developersignup.html).

- **Obtain an Access Token**
  Follow the instructions to generate an access token: [Lightstep Access Token Guide](https://docs.lightstep.com/docs/create-and-manage-access-tokens).


```console
kubectl create ns ais
kubectl create secret generic -n ais lightstep-token --from-literal=token=<YOUR-LIGHTSTEP-TOKEN>
kubectl apply -f config/samples/ais_v1beta1_aistore_tracing.yaml
```

After a successful deployment, traces will be available in the Lightstep dashboard.

While Lightstep is used in the example for simplicity, AIStore supports exporting traces to any OpenTelemetry (OTEL)-compatible tracing solution.

Refer to the [AIStore distributed-tracing](https://github.com/NVIDIA/aistore/blob/main/docs/distributed-tracing.md) doc for more details.

### Locally testing multi-node AIS cluster

By default, AIS operator restricts having more than one AIS target per K8s node.
In other words, if AIS custom resource spec has a `size` greater than the number of K8s nodes, additional target pods will remain pending until we add a new K8s node.

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

### Configure Cloud Providers 

AIS operator supports configuring cloud providers for buckets.
To enable the config for these providers, you need to create a secret with the corresponding credential file.

#### Helm
Helm deployments include a [chart](../helm/ais/charts/cloud-secrets/Chart.yaml) for generating these secrets based on local config and credentials. 
See the [Helm AIS README](../helm/ais/README.md#cloud-credentials) for instructions.

#### Ansible
For ansible deployments, see the [ais_aws_config](../playbooks/cloud/ais_aws_config.yml) and [ais_gcp_config](../playbooks/cloud/ais_gcp_config.yml) playbooks and the associated [README](../playbooks/cloud/README.md).

#### Manual
You can also create the secrets manually:

```bash
kubectl create secret -n ais-operator-system generic aws-creds \
  --from-file=config=$HOME/.aws/config \
  --from-file=credentials=$HOME/.aws/credentials

kubectl create secret -n ais-operator-system generic gcp-creds \
  --from-file=gcp.json=<path-to-gcp-credential-file>.json
```

Once the secrets are created, update the AIS config yaml to reference the secrets:

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

For **GCP** configs, the environment variable for the location may be provided through the `targetSpec.Env` section.
By default, this will be `/var/gcp/gcp.json`
As of writing, the operator will always mount the provided secret to `/var/gcp`, so for a secret with `data.gcp.json` the resulting file location in the pod will be `var/gcp/gcp.json`. 
This is the default value for the `GOOGLE_APPLICATION_CREDENTIALS` environment variable in the container. 

### AIS HTTPS Deployment

You may want to enhance the security of your AIStore deployment by enabling HTTPS.

**Important:** Before proceeding, please ensure that you have `cert-manager` (or equivalent) installed.

To deploy with HTTPS, the AIS spec must define the `spec.ConfigToUpdate.net.http` section, example below: 

```yaml
    net:
      http:
        server_crt: "/var/certs/tls.crt"
        server_key: "/var/certs/tls.key"
        use_https: true
        skip_verify: true # if you are using self-signed certs without trust
        client_ca_tls: "/var/certs/ca.crt"
        client_auth_tls: 0
```

>> Note: This will be included in the spec by default when enabling https and using our Helm charts or playbooks

#### Using a secret mount

If you are using a secret mount to access your certificate, define it with `spec.tlsSecretName`.
The operator will automatically mount the contents of the secret at the location `/var/certs`.

We provide automation for creating this secret for both Helm and Ansible Playbooks. 

Helm: See [HTTPS Deployment docs section](../helm/ais/README.md#https-deployment)

Playbooks: See [generate_https_cert.yml](../playbooks/ais-deployment/generate_https_cert.yml) and associated [templates](../playbooks/ais-deployment/roles/generate_https_cert/templates).

#### Using csi-driver
With `cert-manager csi-driver` installed, you can get signed certificates directly from your Issuer.
The sample configuration below contains definitions for RBAC and an Issuer for use with Vault.

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

For comprehensive testing documentation including unit tests, E2E tests, cluster setup, and test configuration options, see [tests/README.md](tests/README.md).
