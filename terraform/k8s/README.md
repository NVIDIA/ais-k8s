This package is called `k8s` as it assumes that Kubernetes cluster is already running,
and `kubectl` commands can be successfully executed.

Division for different providers exists, because we might want to use different providers for instance of storage.
Probably, it's possible to mount AWS storage to Kubernetes cluster deployed on Google infrastructure.
