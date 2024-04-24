# Compatibility Matrix for AIStore on ais-operator

When possible, the operator maintains backwards compatibility for previous aisnode versions to allow upgrades, but each aisnode version requires a certain operator version. 
The following matrix shows the compatible versions of AIStore ([aisnode](https://hub.docker.com/r/aistorage/aisnode/tags)) with [ais-operator](https://hub.docker.com/r/aistorage/ais-operator/tags).


| AIStore Version | Required Operator Version | Key Enhancements and Notes |
|-----------------|---------------------------|----------------------------|
| v3.21 and below | v0.x                      | Enhancements for cold and warm GET. Added TLS switching. Release notes for [ais](https://github.com/NVIDIA/aistore/releases/tag/v1.3.21) and [ais-operator](https://github.com/NVIDIA/ais-k8s/releases/tag/v0.98). |
| v3.22           | v1.0.0                    | Includes different sizes of proxy/target stateful sets, multi-home support, and enhanced TLS. Features [blob-downloader](https://github.com/NVIDIA/aistore/blob/main/docs/blob_downloader.md). Release notes for [ais](https://github.com/NVIDIA/aistore/releases/tag/v1.3.22) and [ais-operator](https://github.com/NVIDIA/ais-k8s/releases/tag/v1.0.0). |
| v3.23 (latest)  | v1.1.0 (latest)           | Introduces `mountLabel` and improvements for k8s lifecycle operations. Release notes for [ais](https://github.com/NVIDIA/aistore/releases/tag/v1.3.23) and [ais-operator](https://github.com/NVIDIA/ais-k8s/releases/tag/v1.1.0). |


**NOTE:** We recommend and support only the latest versions of AIStore and ais-operator.

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
