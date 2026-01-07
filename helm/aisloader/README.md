# AIS Loader

Helm charts for running coordinated [aisloader](https://github.com/NVIDIA/aistore/blob/main/docs/aisloader.md) benchmarks in Kubernetes.

This Helmfile manages two independent releases:

| Release              | Description                                                                                                           |
|----------------------|-----------------------------------------------------------------------------------------------------------------------|
| `aisloader`          | Benchmark job ([indexed job](https://kubernetes.io/blog/2021/04/19/introducing-indexed-jobs/) with *N* parallel pods) |
| `aisloader-graphite` | Optional metrics visualization ([Graphite](https://graphiteapp.org/) + [StatsD](https://github.com/statsd/statsd))    |

Each release can be managed separately using `-l name=<release>`.

## Usage

```bash
$ helmfile sync -l name=aisloader --set replicas=3 --set params.duration=5m
$ kubectl wait --for=condition=complete job/aisloader -n ais
```

Jobs are immutable. To re-run a benchmark, destroy the previous one first:

```bash
$ helmfile destroy -l name=aisloader
$ helmfile sync -l name=aisloader --set replicas=3 --set params.duration=5m
```

## Collect Results

Each pod outputs JSON stats to `stdout`. Collect and consolidate them:

```bash
$ mkdir -p results
$ for pod in $(kubectl get pods -l job-name=aisloader -n ais -o name); do
    kubectl logs $pod -n ais > results/$(basename $pod).json
  done

$ curl -sLO https://raw.githubusercontent.com/NVIDIA/aistore/main/bench/tools/aisloader-composer/consolidate_results.py
$ python consolidate_results.py results/ put
Processing 3 files from 'results/'
Operation type: put
--------------------------------------------------------------------------------
✓ aisloader-0-vdnpz.json: 5,825 ops, 0.09 GiB/s, 0 errors
✓ aisloader-1-gxq7c.json: 5,577 ops, 0.09 GiB/s, 0 errors
✓ aisloader-2-66rrq.json: 5,704 ops, 0.09 GiB/s, 0 errors

================================================================================
CONSOLIDATED RESULTS
================================================================================
Files Processed:                 3
Total Operations:                17,106
Total Data Transferred:          16.71 GiB
Minimum Latency:                 37.322 ms
Average of Average Latencies:    674.164 ms
Maximum Latency:                 4349.871 ms
Average Throughput:              0.09 GiB/s
Summation of all Throughputs:    0.28 GiB/s
Total Errors:                    0
================================================================================
```

## Metrics

Optionally, install Graphite for real-time metrics visualization:

```bash
$ helmfile sync -l name=aisloader-graphite
```

Then run benchmarks with `graphite.enabled=true` to send metrics:

```bash
$ helmfile sync -l name=aisloader --set graphite.enabled=true --set replicas=3 --set params.duration=5m
```

Access the Graphite dashboard:

```bash
$ kubectl port-forward svc/aisloader-graphite 8080:80 -n ais
```

## Cleanup

To uninstall everything:

```bash
$ helmfile destroy
```
