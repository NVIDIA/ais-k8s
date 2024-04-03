## Operator Testing

This directory provides the ability to test local operator changes on a local cluster.

1. Set up a local Kubernetes cluster. We test with minikube using the docker driver, but other setups should work as well with some slight modifications.

1. Ensure `kubectl` is installed. 

1. Export `MINIKUBE_HOME` to your minikube home (script default is `/var/local/minikube/.minikube`)

1. Run `test_local.sh` to run all tests. Optionally, provide a parameter `short` or `long` to run only those tests. To run a specific test, add the `override` label to the test and run `test-local.sh manual`. This script will: 
    1. Build an operator test image to deploy inside the cluster with all local changes
    1. Configures kubectl to access a minikube cluster
    1. Start a job defined by `test_job_spec.yaml` to run a single-run pod to run operator tests within the cluster. 
