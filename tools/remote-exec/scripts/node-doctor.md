# node-doctor.sh

Wrapper that runs [Node Doctor](https://docs.oracle.com/en-us/iaas/private-cloud-appliance/pca/oke/using-node-doctor-to-troubleshoot-worker-node-issues.htm)
(`/usr/local/bin/node-doctor.sh`), the worker-node diagnostic shipped on OCI worker-node
images, to triage compute-node health (OS, disk, networking, host services). It normally
requires SSH (`opc` + `sudo`); this wrapper `chroot`s into the mounted host root so it runs
with only kubectl/Helm access.

`--check` is read-only. The wrapper always exits 0 (and skips cleanly if Node Doctor is not
installed on the node) so a DaemonSet pod stays healthy and the report is readable via
`kubectl logs`.

## Check all nodes (DaemonSet)

```bash
helm upgrade --install node-doctor . -f values-node-doctor.yaml -n kube-system
for p in $(kubectl get pods -n kube-system -l app=node-doctor -o name); do
  echo "== $p =="; kubectl logs -n kube-system "$p" -c run-script
done
helm uninstall node-doctor -n kube-system
```

## Check a single node

```bash
helm upgrade --install node-doctor . -f values-node-doctor.yaml \
  --set workload.kind=pod --set nodeName=<node-ip> -n kube-system
kubectl logs -n kube-system node-doctor -c ais-remote-exec
helm uninstall node-doctor -n kube-system
```

## Generate a support bundle for Oracle Support

`--generate` writes a bundle to the host `/tmp`. The host root is mounted at `/host`, so it is
reachable at `/host/tmp` from a debug pod and can be streamed out with `kubectl exec ... cat`:

```bash
helm upgrade --install debug . --set nodeName=<node-ip> -n kube-system
kubectl exec -n kube-system debug-privileged -- chroot /host /usr/local/bin/node-doctor.sh --generate
f=$(kubectl exec -n kube-system debug-privileged -- sh -c 'ls -t /host/tmp/oke-support-bundle-*.tar | head -1')
kubectl exec -n kube-system debug-privileged -- cat "$f" > "./$(basename "$f")"   # binary-safe (no TTY)
helm uninstall debug -n kube-system
```
