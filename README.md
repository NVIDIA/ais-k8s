# Deploy AIStore on Kubernetes

The repository includes supporting material for deploying [AIStore](https://github.com/NVIDIA/aistore)
on Kubernetes:
- A Helm chart to install AIStore
- Ansible playbooks to assist in preparing nodes to host AIStore
- Documentation
- A Helm chart for deploying aisloader, for sythetic GET loads
- [Terraform](terraform/README.md) definitions for public cloud usage, such as GKE/GCP.

The repository is split from the main AIStore repo to facilitate GitOps-style deployments, free from
the unrelated commit noise of the development repo.

## Cloud Deployment

If you want to deploy a fresh Kubernetes cluster in the cloud with AIStore, please refer to the
[terraform](terraform/README.md) directory of this repository.

## Small Scale Experimental Deployments

It is assumed you want to deploy AIStore at reasonable scale on multiple nodes each
with multiple drives. If you don't require such scale then consider deploying under Docker
as illustrated in the [main AIStore repo](https://github.com/NVIDIA/aistore).

## Deployment Documentation

Deployment requires some planning and preparation before you can `helm install`.
The [deployment documentation](docs/README.md) walks you through the steps.

## Using This Repository For GitOps-Style Deployment

We suggest cloning this repository and retaining the `master` branch as tracking this upstream `master`; create
a new branch off of master and edit `values.yaml` etc., and point your CD tool at that branch. When
you pull updates to the master you can pull and merge them into your private branch.
