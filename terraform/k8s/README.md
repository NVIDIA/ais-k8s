This package is called `k8s` as it assumes that the Kubernetes cluster is already running,
and `kubectl` commands can be successfully executed.

Division for different providers exists because we might want to use other providers, for instance of storage.
Probably, it's possible to mount AWS storage to the Kubernetes cluster deployed on Google infrastructure.
