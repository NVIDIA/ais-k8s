# Scaling

The operator continuously reconciles the AIS cluster toward its spec.
For scaling, this means keeping the number of proxies and targets equal to the requested size, or, with autoscaling, to the number of nodes that meet the `nodeSelector` and tolerations requirements.

## Target Scale-Down

The target count drops when `size` or `targetSpec.size` is lowered.

A StatefulSet removes pods from the highest ordinal down, so the outgoing targets are the ones at or above the new size.
Before the operator shrinks the StatefulSet, it takes those targets out of the AIS cluster as `targetSpec.scaleDownMode` sets.

### Scale-Down Modes

- `safe_decommission` (default): Moves the data of each outgoing target onto the remaining targets, then decommissions the target and keeps its on-disk data.
- `decommission`: The same as `safe_decommission`, except that the decommission deletes the on-disk data of the target.
- `retain`: Puts each outgoing target in maintenance without moving its data. The data stays on the node and is unreachable until a target with the same ordinal runs there again.

### Decommission Flow

With `decommission` or `safe_decommission`, the operator removes the outgoing targets in four steps:

1. It puts the targets in maintenance, which starts a rebalance that moves their data onto the remaining targets.
2. It waits for AIS to set the post-rebalance flag on each target, which AIS sets only once the rebalance succeeds.
3. It decommissions the targets and marks each pod with the `aistore.nvidia.com/decommissioned` annotation.
4. Once the targets have left the cluster map, it shrinks the StatefulSet.

AIS removes a decommissioned target from the cluster even if its rebalance fails, so the operator uses maintenance to confirm the data moved before it decommissions anything.
See [Node lifecycle](https://github.com/NVIDIA/aistore/blob/main/docs/lifecycle_node.md) for maintenance and decommission in AIS.

### Retain Flow

With `retain`, the operator puts each outgoing target in maintenance without a rebalance, and shrinks the StatefulSet once all of them are in maintenance.
Without a rebalance, the data on the outgoing targets leaves the cluster, so `retain` usually fits when that data can be re-fetched from remote backends.
In this mode, scale-down is faster, but those re-fetches can be slower than in-cluster reads and add cloud-provider egress costs.

### When Scale-Down Waits

In every mode, the operator does not start removing targets while an outgoing pod is missing, still starting, not ready, or still joining the cluster map.
A pod being replaced during a rollout looks the same as a target that has left, so the operator waits for the pod to settle.

With autoscaling, the operator starts removing targets only while the unready targets that the StatefulSet keeps are within `targetSpec.autoScale.maxUnavailable`.
Outgoing targets do not count against it.

With `decommission` or `safe_decommission`, scale-down also waits while a rebalance is running, and while `spec.configToUpdate.rebalance.enabled` is `false`.
For the latter, the operator emits a `Waiting` event on the `AIStore`.

### When Scale-Down Blocks

With `decommission` or `safe_decommission`, two cases stop the scale-down until someone acts.

#### A Target in Maintenance Without the Post-Rebalance Flag

A target can already be in maintenance without the flag, for example if an admin started maintenance without a rebalance, or the rebalance aborted.
AIS does not rebalance when it decommissions a target that is already in maintenance, so decommissioning it would leave its data behind.
The operator logs `waiting for target <pod> in maintenance to set post-rebalance` until the target is taken out of maintenance:

```console
ais cluster add-remove-nodes stop-maintenance <node-id>
```

`<node-id>` is the target's AIS daemon ID.
The next scale-down attempt then puts the target back in maintenance with a rebalance.
See [Incomplete Transitions](https://github.com/NVIDIA/aistore/blob/main/docs/lifecycle_node.md#incomplete-transitions).

#### A Target That Left the Cluster Map Before the Operator Decommissioned It

A target that drops out of the cluster map on its own, for example after a crash, may still hold the only copy of its data.
If its pod has no `aistore.nvidia.com/decommissioned` annotation and a bound data PVC, the operator emits a `Waiting` event and does not remove it.
To continue, bring the target back into the cluster, or set `scaleDownMode` to `retain` to remove it without moving its data.

If the pod has no bound data PVC, scale-down does not wait and the operator removes the pod.
If the target has hostPath mounts, which can still hold data, the operator removes it anyway and emits an `UnconfirmedRemoval` warning event.
