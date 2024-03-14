# Deploying Multiple Storage Targets per Kubernetes Node

For production environments, it's recommended to operate one proxy and one target per Kubernetes (K8s) node. AIStore scales linearly with each added disk (and not target). However, scenarios may arise where deploying multiple targets per node is beneficial. This guide outlines how to configure such deployments using K8s Persistent Volumes (PVs), Persistent Volume Claims (PVCs), and networking options.

## Advantages of Multiple Targets

Deploying multiple targets on a single node can enhance **availability**. For example, a single target with four disks represents a single point of failure. If this target goes down or undergoes maintenance, it could disrupt access to the entire cluster. Conversely, splitting these disks between two targets on the same node increases **resilience**. If one target becomes unavailable, the other can continue serving requests, minimizing downtime.

## Configuring Network Access

In standard deployments, the `hostPort` setting in a pod specification is used to map a container's port to a corresponding port on the host machine. This approach needs adjustment when deploying multiple targets on a single node to avoid port conflicts.

### Internal and External Access

- **Internal Access**: By omitting the `hostPort` field, targets can communicate internally using `servicePort`. This setup restricts external access, safeguarding your cluster from unauthorized external communications.
- **External Access**: If external access to targets is necessary, consider deploying a LoadBalancer. Set `externalLB` to `true` in your StatefulSet specification to facilitate this access.

## Persistent Volume Configuration

Properly setting up PVs and PVCs is crucial for managing storage resources in Kubernetes. Here is an example of a [sample PV](samples/sample-pv.yaml).

### Creating PVs and PVCs

1. **Naming Convention**: Follow a naming convention for PVCs: `<namespace>-<path-separated-by-dashes>-<pod-name>`. For instance, `ais-data-aistore-ais-target-3` breaks down as follows:
   - `ais` represents the namespace.
   - `data/aistore` is transformed to `data-aistore` as the path.
   - `ais-target-3` indicates the specific pod name where the pv is going to be attached.
   
2. **Mount Configuration**: In your StatefulSet's target specification, define mounts as follows:
   ```yaml
   spec:
     ...
     targetSpec:
       mounts:
       - path: /data/aistore
         size: 5T
         storageClass: ais-local-storage
     ...
   ```

3. **Disable Pod Anti-Affinity**: To allow the placement of multiple proxies/targets on a single node, set `disablePodAntiAffinity: true` in your StatefulSet. This adjustment is critical for deploying multiple targets per node efficiently.

## Example Deployment

In this [sample deployment](samples/sample-multi-target-deployment.yml) scenario two PVs are created, each linked to a distinct disk. A cluster is then deployed with one proxy and two targets, each target utilizing one of the PVs.
