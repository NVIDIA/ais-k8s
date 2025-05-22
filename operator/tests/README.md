# Operator Testing

Run all commands from the parent `operator` directory.

## Unit Tests

To run unit tests:

```bash
$ make test
```

## E2E Tests

E2E tests require an existing Kubernetes cluster.

### Supported Environment Variables

The following environment variables allow customization of the E2E test environment:

| Variable Name             | Description                                                | Default Value                                   |
|---------------------------|------------------------------------------------------------|-------------------------------------------------|
| `AIS_TEST_NODE_IMAGE`     | Node container image used for deploying AIS nodes          | `aistorage/aisnode:v3.29`                       |
| `AIS_TEST_PREV_NODE_IMAGE`| Previous node image for upgrade/downgrade testing          | `aistorage/aisnode:v3.28`                       |
| `AIS_TEST_INIT_IMAGE`     | Init container image used during AIS deployment            | `aistorage/ais-init:v3.29`                      |
| `AIS_TEST_PREV_INIT_IMAGE`| Previous init container image for upgrade/downgrade testing| `aistorage/ais-init:v3.28`                      |
| `AIS_TEST_API_MODE`       | API mode used for non-external LB clusters                 | Internal DNS (headless service)                 |
| `TEST_STORAGECLASS`       | Storage class to use for test volumes                      | `ais-operator-test-storage` (`standard` for GKE)|
| `TEST_STORAGE_HOSTPATH`   | Host path to use for storage when using hostPath volumes   | `/etc/ais/<random>`                             |
| `TEST_EPHEMERAL_CLUSTER`  | Indicates testing on ephemeral clusters                    | `false`                                         |

### Create Cluster

You can create a local Kubernetes cluster using [`kind`](https://kind.sigs.k8s.io/), a tool for running Kubernetes clusters in Docker containers. 

> **NOTE:** Before using `kind`, ensure you have either [Docker](https://docs.docker.com/get-docker/) or [Podman](https://podman.io/getting-started/installation) installed on your system.

To create the cluster:

```bash
$ make kind-setup
```

### Run Tests

Before running E2E tests, bootstrap the test environment (e.g. install dependencies, start required background services such as `cloud-provider-kind`):

```bash
$ make test-e2e-bootstrap
```

Then, you can run tests in the cluster:

```bash
$ make test-in-cluster
```

Or, run tests outside the cluster:

```bash
$ AIS_TEST_API_MODE=public make test-e2e
```

> **NOTE:** The test suite uses the cluster's internal DNS service by default; set `AIS_TEST_API_MODE=public` to use the public API endpoint when running outside the cluster.

To run specific E2E tests marked with the `manual` [label](https://onsi.github.io/ginkgo/#spec-labels), set the `TEST_E2E_MODE` environment variable:

```bash
$ TEST_E2E_MODE=manual make test-e2e-in-cluster
```

### Cleanup

To tear down the test environment (e.g. uninstall test dependencies, stop background `cloud-provider-kind` service):

```bash
$ make test-e2e-teardown
```

To delete the cluster entirely:

```bash
$ make kind-teardown
```
