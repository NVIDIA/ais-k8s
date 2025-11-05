# Compatibility Matrix for AIStore on ais-operator

> **WARNING:** Upgrading the operator version from < v2.0 to any version v2.0 or later WILL cause an AIS cluster restart.

When possible, the operator maintains backwards compatibility for previous AIStore versions to allow upgrades, but each aisnode version requires a certain operator version. 
The following matrix shows the compatible versions of AIStore ([aisnode](https://hub.docker.com/r/aistorage/aisnode/tags)) with [ais-operator](https://hub.docker.com/r/aistorage/ais-operator/tags).


| AIStore Version | Required Operator Version         | Compatibility Notes                                                                                                 | Release Notes                                                                                                                                                                                                                                           |
|-----------------|-----------------------------------|---------------------------------------------------------------------------------------------------------------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| v3.21 and below | v0.x                              |                                                                                                                     | [AIS](https://github.com/NVIDIA/aistore/releases/tag/v1.3.21), [operator](https://github.com/NVIDIA/ais-k8s/releases/tag/v0.98)                                                                                                                         |
| v3.22           | v1.0.0                            |                                                                                                                     | [AIS](https://github.com/NVIDIA/aistore/releases/tag/v1.3.22), [operator](https://github.com/NVIDIA/ais-k8s/releases/tag/v1.0.0)                                                                                                                        |
| v3.23           | v1.1.0                            |                                                                                                                     | [AIS](https://github.com/NVIDIA/aistore/releases/tag/v1.3.23), [operator](https://github.com/NVIDIA/ais-k8s/releases/tag/v1.1.0)                                                                                                                        |
| v3.24           | v1.4.1                            | Operator > v1.4.x required to transition from AIS v3.23 to v3.24                                                    | [AIS](https://github.com/NVIDIA/aistore/releases/tag/v1.3.24), [operator v1.4.0](https://github.com/NVIDIA/ais-k8s/releases/tag/v1.4.0), [operator v1.4.1](https://github.com/NVIDIA/ais-k8s/releases/tag/v1.4.1)                                       |
| v3.25           | v1.4.1                            | Operator transitioning to init-managed config, see below                                                            | [AIS](https://github.com/NVIDIA/aistore/releases/tag/v1.3.25), [operator v1.5.0](https://github.com/NVIDIA/ais-k8s/releases/tag/v1.5.0)                                                                                                                 |
| v3.26           | v1.6.0 (latest >v2.0 recommended) | Requires init container compatible with v3.26                                                                       | [operator v1.6.0](https://github.com/NVIDIA/ais-k8s/releases/tag/v1.6.0)                                                                                                                                                                                |
| v3.27           | v1.6.0                            |                                                                                                                     | [AIS](https://github.com/NVIDIA/aistore/releases/tag/v1.3.27),[operator v2.2.0](https://github.com/NVIDIA/ais-k8s/releases/tag/v2.2.0)                                                                                                                  |
| v3.28           | v1.6.0                            | Operator >= v2.3.0 **NOT** compatible with AIS < v3.28 <br/>(recommend upgrade to > v3.28 BEFORE operator > v2.3.0) | [AIS](https://github.com/NVIDIA/aistore/releases/tag/v1.3.28)                                                                                                                                                                                           |
| v4.1            | v2.8.0                            | AIS ≥ v4.1 is **NOT** compatible with operator < v2.8.0 (upgrade operator to > v2.8.0 first); AIS now gates readiness on cluster join, requiring `publishNotReadyAddresses: true` on headless SVCs for peer discovery during startup | [AIS](https://github.com/NVIDIA/aistore/releases/tag/v1.4.1), [operator v2.8.0](https://github.com/NVIDIA/ais-k8s/releases/tag/v2.8.0) |

**NOTE:** We recommend and support only the latest versions of AIStore and the AIS K8s Operator.

## Init container compatibility
Starting with operator version `1.6.0`, we have begun to move the generation of AIS config from the operator to the init container.

These init containers will now be versioned alongside `aisnode` and should be updated alongside aisnode and kept in sync. 

Older clusters can be upgraded to an operator with version `1.6.0`, however `1.6.0` can **NOT** deploy new clusters with an init container using the old versioning system.

As of version 2.0, support of any `v1.*` init container has been dropped and the init container version must match the aisnode version. 

We recommend upgrading to the latest compatible init version directly after upgrading the operator to `1.6.0`.

| Operator Version | Init Container Version | AISNode Version |
|------------------|------------------------|-----------------|
| 1.5.0            | v1.2.0, v3.25          | v3.25           |
| 1.6.0            | v3.25                  | v3.25           |
| 1.6.0            | v3.26                  | v3.26           |
| 2.0 or later     | ...                    | ...             |


## Updating the ais-operator image

**If you don't have the operator running:**
Follow the steps in the [deployment docs](README.md#operator-deployment-procedure).

**If you already have the operator running and want to update the image:**
```console
$ # edit release_version
$ kubectl apply -f https://github.com/NVIDIA/ais-k8s/releases/download/{release_version}/ais-operator.yaml
```

## Updating the aisnode image

**If you don't have a cluster running:**
Follow these [instructions](README.md#aistore-cluster-creation-process).

**If you already have a cluster running and want to update the aisnode image:**
You can either edit the `aistores` resource:
```console
$ # edit the nodeImage field
$ kubectl edit aistores -n ais
```
Or, use a patch command:
```console
$ kubectl patch aistores ais -n ais --type=merge -p '{"spec": {"nodeImage": "aistorage/aisnode:v3.23"}}'
```

**Note:** Make sure to update the operator before updating the aisnode.


Please ensure you are using compatible versions when deploying AIStore with ais-operator.
