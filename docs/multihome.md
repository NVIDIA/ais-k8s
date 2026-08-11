# Multihome Deployment

To take advantage of multiple network interfaces, AIS supports multi-homing to distribute traffic across all available interfaces.
The operator currently supports using [multus](https://github.com/k8snetworkplumbingwg/multus-cni) to enable multiple IPs for each K8s pod.

> **Note:** Using more than 2 network interfaces has not been tested. Refer to the multus documentation or raise an issue if you run into errors assigning multiple IPs to a pod.

## Host Prerequisites

Each interface must have its own IP, reachable from every other node in the K8s cluster.
All nodes must be able to connect to all interfaces on all other nodes.

The cluster needs multus to attach the second interface to each pod, and [whereabouts](https://github.com/k8snetworkplumbingwg/whereabouts) to assign IPs from it.
Install both before deploying AIS, following the [multus installation guide](https://github.com/k8snetworkplumbingwg/multus-cni/blob/master/docs/quickstart.md#installation) and the [whereabouts installation guide](https://github.com/k8snetworkplumbingwg/whereabouts#installation).

## Network Attachment Definitions

A `NetworkAttachmentDefinition` tells multus how a pod reaches an additional interface.
One must exist in the AIS namespace for each interface you want to use.

A macvlan bridge sample is provided at [manifests/multus/nad-macvlan.yaml](../manifests/multus/nad-macvlan.yaml).
Set `master` to the host interface, `range` to the CIDR pool reserved for pod IPs on that interface, and `gateway` to the gateway address for that pool.
For more than one additional interface, apply a copy per interface, each with its own `metadata.name`.

```console
kubectl apply -f manifests/multus/nad-macvlan.yaml
```

For other CNI types, refer to the [multus documentation](https://github.com/k8snetworkplumbingwg/multus-cni/blob/master/docs/how-to-use.md#create-network-attachment-definition).

Check which definitions exist in your namespace:

```console
kubectl get network-attachment-definitions -n <namespace>
```

## Configuring AIS

Two spec entries connect the cluster to those definitions:

1. **`spec.networkAttachment`:** Comma-separated names of the definitions to attach to every pod.
2. **`spec.hostnameMap`:** Maps each node's primary host to a comma-separated list of all its hosts, including the primary.

The operator writes `hostnameMap` to a ConfigMap and passes it to the `populate-env` init container as `-hostname_map_file`.
That container is [ais-init](https://github.com/NVIDIA/aistore/blob/main/cmd/aisinit/main.go#L84-L94), which looks up the pod's primary host in the map and writes the matching list to `host_net.hostname` in the pod's local config.
AIS itself never reads the map, so the primary host is only ever a lookup key.

> **Note:** On a TLS cluster the operator also adds every host in the map to the generated certificate's SANs.
> An address left out of the map is left out of the certificate, and clients reaching a node on that address fail verification.

```yaml
spec:
  networkAttachment: "macvlan-conf"
  hostnameMap:
    10.51.248.68: "10.51.248.68,10.51.248.116"
    10.51.248.77: "10.51.248.77,10.51.248.105"
```

For Helm, set these under the `multihome` block in your AIS values file and refer to the [AIS Helm multihome section](../helm/ais/README.md#multihome).

## Verifying

Run `ais show cluster smap --json`.
Each node should carry its additional IP under `pub_extra`:

```json
"public_net": {
    "node_ip_addr": "10.51.248.68",
    "daemon_port": "51081",
    "direct_url": "http://10.51.248.68:51081"
},
"pub_extra": [
    {
        "node_ip_addr": "10.51.248.116",
        "daemon_port": "51081",
        "direct_url": "http://10.51.248.116:51081"
    }
],
```

## Troubleshooting

If pods fail to initialize, or `pub_extra` is missing from the cluster map:

- Check that the network attachment definitions exist in the AIS namespace:

  ```console
  kubectl get network-attachment-definitions -n <namespace>
  ```

- Check `host_net.hostname` in the pod's generated local config.
  [ais-init](https://github.com/NVIDIA/aistore/blob/main/cmd/aisinit/main.go#L84-L94) resolves it from the `hostnameMap` entry whose key matches the pod's primary host, so a key that does not match leaves the additional addresses out.

  ```console
  ais show config <node-id> local host_net
  ```

  `host_net.hostname` should list every address for that node, not just the primary.

- Check the `populate-env` container logs for failures.

  ```console
  kubectl logs -n <namespace> <failing ais pod> -c populate-env
  ```
