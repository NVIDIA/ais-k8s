# AIS Operator Cluster States

This document describes the cluster lifecycle states managed by the AIS Kubernetes operator.

## Cluster States

| State | Description |
|-------|-------------|
| `Initialized` | Cluster CR is created with finalizer, but not yet provisioned |
| `InitializingLoadBalancerService` | Setting up external LoadBalancer service (if enabled) |
| `PendingLoadBalancerService` | Waiting for LoadBalancer external IP allocation |
| `Created` | Basic resources deployed, cluster bootstrapping complete |
| `Ready` | Cluster is fully operational and ready for workloads |
| `Upgrading` | Cluster is applying configuration, spec, or scaling changes |
| `ShuttingDown` | Graceful shutdown in progress (preserves data) |
| `Shutdown` | Cluster is scaled to zero replicas (data preserved on disk) |
| `Decommissioning` | CR deleted, calling AIS shutdown/decommission API |
| `CleaningResources` | Deleting K8s resources (StatefulSets, Services, ConfigMaps) |
| `HostCleanup` | Running cleanup jobs for hostpath state mounts |
| `Finalized` | Cleanup complete, removing finalizer for CR deletion |
