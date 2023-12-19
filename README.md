# AIStore on Kubernetes

The repository contains tools and supporting materials for deploying [AIStore](https://github.com/NVIDIA/aistore) on Kubernetes.

This includes:

- Ansible playbooks to assist in preparing nodes to host AIStore
- [Kubernetes operator](operator/README.md)
- [Documentation](/docs), and in particular:
  - [Kubernetes Operator Deployment: Steps](docs/walkthrough.md)
- [Terraform](terraform/README.md) definitions for public cloud usage, such as GKE/GCP.

## Cloud Deployment

If you want to deploy a fresh Kubernetes cluster in the cloud with AIStore, please refer to the
[terraform](terraform/README.md) directory of this repository.

## Small Scale Experimental Deployments

It is assumed you want to deploy AIStore at reasonable scale on multiple nodes each
with multiple drives. If you don't require such scale then consider deploying under Docker
as illustrated in the [main AIStore repo](https://github.com/NVIDIA/aistore).

## Deployment Documentation

You can deploy AIStore on Kubernetes in two ways. In both cases, some preparation and planning is needed;
we suggest you read the [deployment documentation](docs/README.md) first.

### Deployment via the AIStore Operator
AIStore is deployed using the [AIStore operator](operator/README.md).

With an operator based deployment, instead of deploying services directly, you define your AIStore
cluster as a [kubernetes custom resource](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/).

The operator documentation can be found [here](operator/README.md), along with detailed [walkthrough](docs/walkthrough.md) guidance.

## Using This Repository For GitOps-Style Deployment

We suggest cloning this repository and retaining the `main` branch as tracking this upstream `main`; create
a new branch off of main and edit `values.yaml` etc., and point your CD tool at that branch. When
you pull updates to the main you can pull and merge them into your private branch.
