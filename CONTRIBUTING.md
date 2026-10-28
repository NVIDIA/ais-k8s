# Contributing to AIStore

This repository follows an open source model where anyone is allowed and encouraged to contribute.
However, contributing to AIStore has a few guidelines that must be followed.

## Contribution Workflow

The AIS-K8s repository maintains a contribution structure in which everyone *proposes* changes to the codebase via *pull requests*.
To contribute to AIStore:

1. [Fork the repository](https://docs.github.com/en/pull-requests/collaborating-with-pull-requests/proposing-changes-to-your-work-with-pull-requests/creating-a-pull-request-from-a-fork)
2. [Create a new branch](https://docs.github.com/en/issues/tracking-your-work-with-issues/creating-a-branch-for-an-issue)
3. [Test changes](#testing-changes)
4. [Format changes](#formatting-operator-changes)
5. [Update changelog](#update-changelog)
6. [Update documentation](#documentation) in the same commit as a behavior change
7. [Commit changes (w/ sign-off)](#signing-off-commits)
8. [Squash commits](#squashing-commits)
9. [Create a pull request](https://docs.github.com/en/pull-requests/collaborating-with-pull-requests/proposing-changes-to-your-work-with-pull-requests/creating-a-pull-request-from-a-fork)

### Formatting Operator Changes

AIStore operator maintains a few formatting rules to ensure a consistent coding style.
These rules are checked and enforced by `gofmt`, etc.
Before committing any changes, make sure to check (or fix) all changes against the formatting rules as follows:

```console
$ cd operator

# Run linter on entire codebase
$ make lint

# Check code formatting
$ make fmt-check

# Fix code formatting
$ make fmt-fix
```

### Testing Operator Changes

Before committing any changes, run the following tests to verify any added changes to the codebase.
Refer to operator [Development](operator/README.md#development) and [Test](operator/README.md#testing) sections for more details.

### Documentation

These rules apply to all documentation in the repository, including `docs/` and component READMEs.

**Scope**

- Explain operator and AIS behavior: what the operator creates or reconciles, and how AIS responds to each setting.
- Do not explain Kubernetes internals. When a Kubernetes concept is needed, link to the [Kubernetes documentation](https://kubernetes.io/docs/).
- Do not restate content from the [AIStore documentation](https://github.com/NVIDIA/aistore/tree/main/docs). Link to it.
- Do not document internals that change often, such as precedence or fallback order, unless the reader needs them to configure the feature.

**Structure**

- Give each fact one home. Before writing, search the existing documentation for it. If it exists, link to its heading instead of copying it.
- Give each feature that spans several settings, resources, or components its own document. Examples are `docs/tls.md`, `docs/authn.md`, `docs/multihome.md`, and `docs/node_replacement.md`. Other documents summarize it in one or two sentences and link to it.
- Link every new document from [`docs/README.md`](./docs/README.md). Update the links when a document moves or is removed.
- Keep long manifests out of the text and link to them instead.
  - Put custom resource examples, such as `AIStore` and `AIStoreAuth`, in `operator/config/samples/`.
  - Put other resource or configuration samples in `docs/samples/`.
- Keep documented config fields and values that are still supported. Do not remove a deprecated one. Mark it with a `> **Deprecated:**` note that links to its replacement.

**Style**

- Explain what each setting or action does. An imperative is acceptable when the same sentence states the result, such as "Set `spec.targetSpec.disablePodAntiAffinity: true` to allow the operator to place several targets on one node".
- Do not write a sequence of instructions or a list of settings to configure, unless the document is a guide for one task or mode. Write other documents so that a new option does not require a change to them.
- A guide covers the main path of its task only. Link to the feature document for options and variants. Do not enumerate every option, variant, or edge case.
- Do not write verification checklists, "stop if" conditions, or "do not assume" statements. Warn about a risk when an action has a significant negative effect, such as data loss, an outage, or a change that cannot be undone. State the warning as the consequence.
- Show commands and manifests in code blocks. Introduce each with a sentence that states the result.
- Name config fields by their full path in backticks, such as `spec.targetSpec.disablePodAntiAffinity`.
- Write in plain, direct English, following the spirit of [ASD-STE100 Simplified Technical English](https://www.asd-ste100.org/). Prefer the active voice, and use the same word for the same thing.
- Write each sentence of regular prose on its own line. Bulleted text is exempt.
- Write complete sentences, not run-on sentences. Split a sentence that states more than one idea. Use semicolons and em dashes sparingly.
- Be concise. Include only what the reader needs to act.
- Match the style, heading structure, and level of detail of the surrounding file.

### Signing-Off Commits

All contributors must *sign-off* on each commit.
This certifies that each contribution is that contributor's original work per the following *Developer Certificate of Origin*[^developer-certificate-of-origin].

[^developer-certificate-of-origin]: **Developer Certificate of Origin**
    ```
    Developer Certificate of Origin
    Version 1.1

    Copyright (C) 2004, 2006 The Linux Foundation and its contributors.
    1 Letterman Drive
    Suite D4700
    San Francisco, CA, 94129

    Everyone is permitted to copy and distribute verbatim copies of this
    license document, but changing it is not allowed.


    Developer's Certificate of Origin 1.1

    By making a contribution to this project, I certify that:

    (a) The contribution was created in whole or in part by me and I
        have the right to submit it under the open source license
        indicated in the file; or

    (b) The contribution is based upon previous work that, to the best
        of my knowledge, is covered under an appropriate open source
        license and I have the right under that license to submit that
        work with modifications, whether created in whole or in part
        by me, under the same open source license (unless I am
        permitted to submit under a different license), as indicated
        in the file; or

    (c) The contribution was provided directly to me by some other
        person who certified (a), (b) or (c) and I have not modified
        it.

    (d) I understand and agree that this project and the contribution
        are public and that a record of the contribution (including all
        personal information I submit with it, including my sign-off) is
        maintained indefinitely and may be redistributed consistent with
        this project or the open source license(s) involved.
    ```

Commits can be signed off by using the `git` command's `--signoff` (or `-s`) option:

```bash
$ git commit -s -m "Add new feature"
```

This will append the following type of footer to the commit message:

```
Signed-off-by: Your Name <your@email.com>
```

> **Note**: Commits that are not signed-off cannot be accepted or merged.


### Update Changelog

If a contribution updates the [AIS K8s Operator](./operator/README.md), an associated update should also be made to the [changelog](./operator/CHANGELOG.md) in the same commit.

Changelog entries should be short and focused on user-facing impacts of the change. 

New changes should be placed under the current `Unreleased` section and in the appropriate sub-section (e.g. `Added`, `Changed`, `Deprecated`, `Removed`).

### Squashing Commits

If a pull request contains more than one commit, [squash](https://docs.github.com/en/pull-requests/collaborating-with-pull-requests/incorporating-changes-from-a-pull-request/about-pull-request-merges) all commits into one.

The basic squashing workflow is as follows:

```console
git checkout <your-pr-branch>
git rebase -i HEAD~<# of commits to squash>
```

---

## Raise an Issue

If a bug requires more attention, raise an issue [here](https://github.com/NVIDIA/ais-k8s/issues).
We will try to respond to the issue as soon as possible.

Please give the issue an appropriate title and include detailed information on the issue at hand.

---
