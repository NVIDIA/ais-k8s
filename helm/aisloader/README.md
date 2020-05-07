# aisloader Chart

## Introduction

The [aisloader](https://github.com/NVIDIA/aistore/tree/master/bench/aisloader) benchmark utility
can be used to generate extreme GET and PUT workloads against an AIStore instance.

The usual mode of operation is to simulate deep learning training iterations over a
bucket: 100% GET load, full object reads, randomized object access order, parallel
operation, read all objects in bucket before repeating. After reading an object
the client simply drops the data and reads another - there's no computation as
would be the case on a DL workload so we're simulating an infinitely quick GPU.

Aisloader can be run against an existing dataset, or it can generate objects for
later GET runs.

Aisloader can easily be run from the command line of a suitable client. This chart
automates deployment across multiple client nodes.

## Installation

The chart deploys an aisloader benchmark execution framework that will respond
to configuration updates and initiate benchmark runs as requested. The expectation
is that the framework is installed and left to run long-term, with benchmarks
triggered on request as described below. In our reference configuration, this
framework runs within the same k8s as AIStore and executes on nodes that are not
hosting the AIStore cluster itself; when we perform a rolling update on the
AIStore cluster we can trigger benchmark runs against it by updating the
configuration of this aisloader deployment.

You will need to edit/override the following from `values.yaml` before deployment:
- `image.*` to point to the `aisloader` [container image](https://github.com/NVIDIA/aistore/tree/master/deploy/prod/k8s/helm/aisloader_stress/build_docker); if a pull secret is required then provide the secret name and manually pre-populate the secret in the intended k8s namespace
- `ais_release` and `ais_namespace` to nominate the AIStore instance name in this k8s cluster to target; e.g. if you performed `helm install` of AIStore with named `e10` and namespace `ais` quote those
- `controller.results_pvc` - leave empty for EmptyDir semantics, otherwise provide an existing PVC we can use for storing results of all runs requested over time
- `controller.runid` - defaults to `nil` meaning "await instructions via config update", and the recommended way to deploy is with value `nil` and then update config on demand

The chart deploys a DaemonSet with node selection controlled by node label
`aisloader=<AIStore release to target>`. All daemon pods so created register
with the controller pod to form a pool of eligible workers, waiting to run a
multinode benchmark when requested; when a daemon completes an `aisloader`
run task it reports its results and then exits - on the DaemonSet pod restart
it rejoins the worker pool awaiting a future run.

## Operation

### Model

At initial install the chart creates a transient Redis instance to coordinate benchmark
client nodes. A controller pod is created which will monitor configuration and initiate
new benchmark runs when a new `controller.runid` is submitted - the benchmark will run
with the number of nodes and client config as stipulated in the config at the time the new
`runid` is observed. If `runid` is `nil` (the default) then the controller pod will poll
for updates of the config until it observes non-nil and initiates a new run.

A DaemonSet is used to create benchmark client pods. Each daemon pod registers with the
controller pod and enters a "pool" of available nodes. When a new run is requested the
controller will await the availabler pool being large enough to cover the requested
nodecount then will choose the first `nodecount` nodes from a sorted list of available
nodes, hence trying always to run on the same set of nodes where possible for a given nodecount.

Once there are enough nodes to cover the request, the controller asks each node to
start the `aisloader` client and monitors each for completion or failure. As nodes
complete they return their results to the controller which preserves them in the
results volume. Nodes recycle their daemon pod on completion and rejoin the available
pool for future runs.

The controller pod can only orchestrate *one* benchmark run at a time! Requesting another
or updating configuration mid-run will either be ignored or result in malfunction.

The controller will not repeat a `runid`. You should add some form of timestamp or similar
to make them unique. The Redis instance runs with data on an `EmptyDir` volume which will
survive pod restarts but will be destroyed when the instance is torn down. Past `runid` values
are recorded in Redis (along with all other controller state).

Restarting the controller will lead to malfunction - while it stores state in Redis it
does not yet resume such state on restart.

### Edit `values.yaml`

- choose a `controller.runid` that is unique for this run; if the controller pod has attempted a run for the given `runid` before it will reject duplicates
- set `controller.nodecount` to the number of nodes to participate in the benchmake, each running an `aisloader` instance; you will have to have a suitable number of nodes labeled to have a daemon pod pool big enough to match your requested nodecount (the controller pod will wait for sufficient registered worker nodes)
- tweak `config.*` to influence benchmark run parameters such as bucket to target, GET vs PUT mix, number of parallel workers per instance, run duration, etc. Comments in `values.yaml` describe each option.

### Reinstall vs Update

You can choose to delete the `aisloader` application after each benchmark run - just
edit `values.yaml` as needed (make sure to choose a `runid` other than `nil`) and install,
no need to think about requesting additoinal runs from the same installation as detailed
below.


### Aside: Helm CLI vs Continuous Deployment Tools

Configuration updates - to request new runs, change run parameters etc - are made by
editing or over-riding the `values.yaml` file and applying the updates (you can also edit the
generated ConfigMap but it is less than friendly since it's bash script with substitutions
from values).

As such, using Helm CLI options `--set` and `--set-string` to over-ride `values.yaml` to
specify the required values listed above alogn with controlling benchmark request is
likely to be cumbersome and error-prone. Edit `values.yaml` directly and apply the
changes with `helm upgrade`.

If using a CD tool such as Argo CD then control benchmark runs through updates to `values.yaml`.

### Apply `values.yaml`

Apply using one of `helm install`, `helm upgrade` or via CD tool. We use ArgoCD with automatic
Sync.

## Sample Client Configs

### Generate a Bucket of 1G Objects

Since `aisloader` doesn't interpret the data is reads there is no requirement for properly formaed WebDataset shard or similar,
so we can use a 100% PUT load to generate some data. You'll need to have enough data to overwhelm DRAM cache across the
nodes of the AIStore cluster - otherwise we're testing the rate at which we can serve from DRAM when we run the GET test
with this data.
Item | Comment
---- | -------
`controller.nodecount: 8` | 8 nodes will run at once
`config.bucket.default: test1g` | bucket to generate data in
`config.duration.default: 7h` | Time for each client to run; could instead limit data volume  with `maxputs`
`config.pctput: 100` | All PUT activity for now
`config.minsize.default: 1G` | Min object size to generate
`config.maxsize.default: 1G` | Max object size to generate
`config.numworkers.default: 50` | Number of worker threads per client. With 8 nodes this means 400 total

If generating small objexts you could also consider `config.putshards.default: 1000` which would create objects with a siz character prefix "%06x/..." so reducing the number of objects per directoy -
some filesystems don't copy with 10s of millions of files per directory.

With HDD, to be kind, arrange that `nodecount * numworkers` is a low multiple of the total disk count in the cluster. That said, the above 400 worker run against a 120 disk cluster generated 368944 x 1G objects in 7h for a total of 360.3TiB. That's 14.6GiB/s generated, or ~122MB/s per HDD.

### Simulate Streaming 1G Shards to DataLoaders

We'll consume the bucket generated above in a pattern to match the following DL config:
- 8 x 8 GPU nodes (so we run with `nodecount` of 4)
- 5 PyTorch DataLoader workers per GPU each streaming a random 1G shard (so we run 40 `numworkers` per node)
- 100% GET, random permutation of entire object list not re-reading an object until we've completed an epoch through the entire bucket.

Aisloader will drop data as it reads it but, of course, a real DL job would include data
augmentation in the DataLoaders (consuming CPU) and model execution on the GPUs. On
other words, real DL cannot consume data *faster* than `aisloader`.

Item | Comment
---- | -------
`controller.nodecount: 8` | 8 nodes will run at once
`config.numworkers.default: 40` | Number of worker threads per client. With 8 nodes this means 400 total
`config.bucket.default: test1g` | bucket to consume data from
`config.duration.default: 2h` | Time for each client to run
`config.pctput.default: 0` | All GET activity now
`config.uniquegets.default: true` | The default, anyway - client will perform full epoch style passes of the bucket

Leave `readlen` at the default of 0 to read full objects.