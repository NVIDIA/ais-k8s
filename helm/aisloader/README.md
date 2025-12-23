# AIS Loader

This chart creates an [indexed job](https://kubernetes.io/blog/2021/04/19/introducing-indexed-jobs/) with *N* parallel pods. Each pod:

1. Gets a unique index (0 to N-1) via `JOB_COMPLETION_INDEX`
2. Runs `aisloader` with `-loaderid=$INDEX -loadernum=N` for coordinated object naming
3. Outputs JSON stats to stdout (captured in pod logs)

By default, anti-affinity ensures **one loader per node**. Set `replicas` to match your desired benchmark node count.

## Usage

```bash
$ helmfile sync --set replicas=3 --set params.bucket=ais://bench --set params.duration=1m --set params.pctput=100

$ kubectl wait --for=condition=complete job/aisloader -n ais
```

## Collect Results

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

## Cleanup

Jobs auto-delete after configured TTL (default is 24h). To delete immediately:

```bash
$ helmfile destroy
```
