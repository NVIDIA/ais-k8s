# AIStore k8s Deployment

The repository includes supporting material for deploying [AIStore](https://github.com/NVIDIA/aistore)
on Kubernetes:
- a Helm chart to install AIStore
- Ansible playbooks to assist in preparing nodes to host AIStore
- documentation
- A Helm chart for deploying aisloader, for sythetic GET loads

The repository is split from the main AIStore repo to facilitate GitOps-style deployments, free from
the unrelated commit noise of the development repo.

## Small Scale Experimental Deployments
It is assumed you want to deploy AIStore at reasonable scale, such on on multiple nodes each
with multiple drives. If you don't require such scale then consider deploying under Docker
as illustrated in the [main AIStore repo](https://github.com/NVIDIA/aistore).

## Deployment Documentation
Deployment requires some planning and preparation before you can `helm install`.
The [deployment documentation](docs/README.md) walks you through the steps.

## Using This Repository For GitOps-Style Deployment

We suggest cloning this repository and retaining the `master` branch as tracking this upstream `master`; create
a new branch off of master and edit `values.yaml` etc in there, and point your CD tool to that branch. When
you pull updates to the master you can pull and merge them into your private branch.