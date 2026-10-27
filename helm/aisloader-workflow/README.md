# AIS Loader Workflows

Helm charts for running [aisloader](https://github.com/NVIDIA/aistore/blob/main/docs/aisloader.md) benchmarks as [Argo Workflows](https://argoproj.github.io/workflows/).

> **Work in progress.** Charts, templates, and usage are subject to change.
> The list step needs an `ais-util` image with aisloader `-list-out` support.
> Until a released `ais-util` image includes it, set `images.ais-util` to an image built from the AIStore `main` branch.

This Helmfile manages two releases in the `argo` namespace:

| Release                        | Description                        |
|--------------------------------|------------------------------------|
| `argo-workflows`               | Argo Workflows controller          |
| `aisloader-workflow-templates` | `aisloader-bench` WorkflowTemplate |

Settings for a benchmark live in three places:

- [Helm values](charts/workflow-templates/values.yaml) describe the environment, such as the AIS endpoint, TLS, and AuthN. Set them once at install.
- The `config` parameter holds the aisloader config. Set each time the workflow is submitted.
- The other workflow parameters decide which steps run. Set them for each submit.

## Usage

### Install

1. Install Argo Workflows and the `aisloader-bench` WorkflowTemplate. This also creates the `argo` namespace.

    ```bash
    $ helmfile sync
    ```

2. If the AIS cluster serves HTTPS and additional CA trust must be configured, add the AIS CA to the `argo` namespace as the ConfigMap named by `tls.caConfigMap`.

3. If the AIS cluster has authentication enabled, create the AuthN Secret the workflow can use to log in:

    ```bash
    $ kubectl -n argo create secret generic aisloader-authn-creds \
        --type=kubernetes.io/basic-auth \
        --from-literal=username=<user> --from-literal=password=<password>
    ```

### TLS and AuthN

The TLS and AuthN values must match how the AIS cluster is deployed, with or without [HTTPS](../../docs/tls.md) and [authentication](../../docs/authentication.md).

When the AIS cluster serves HTTPS, the workflow pods verify it with the `ca.crt` key of the `aistore.nvidia.com` ConfigMap in the workflow namespace.
The pods use the same CA to verify AuthN.
Set `tls.caConfigMap` and `tls.bundleFile` to use a different ConfigMap and key.
Set `tls.enabled` to `false` when the cluster serves HTTP.
Leave `tls.caConfigMap` empty when the cluster uses a publicly trusted certificate.

When `authn.enabled` is `true` (default), each workflow pod logs in to [AuthN](../../docs/authn.md) before it runs.
The pods log in as the user in the Secret named by `authn.secretName`.
That user needs access to every bucket the workflow uses, such as the benchmark bucket and the bucket the list is written to.

### Submit

Run `aisloader-bench` and write the names of the objects in `ais://get-test` to `ais://lists/run-1.txt`:

```bash
$ printf 'bucket: ais://get-test\n' > conf.yaml
$ argo -n argo submit --from workflowtemplate/aisloader-bench \
    -p config="$(cat conf.yaml)" -p listOut=ais://lists/run-1.txt --wait
```

## Parameters

| Parameter | Default  | Description                                                                   |
|-----------|----------|-------------------------------------------------------------------------------|
| `config`  | required | aisloader config file contents, passed to aisloader with `-config`.           |
| `listOut` | `""`     | Object URI to write the object list to. Runs the [list step](#list) when set. |

## Steps

Each optional step runs only when its parameter is set.

### List

The list step runs aisloader with `-list-out` and writes the object names to `listOut`.
The `listOut` bucket must already exist, and the list step overwrites an existing `listOut` object.

The listing options in `config` decide which objects are in the list.
See [Writing an object-name list](https://github.com/NVIDIA/aistore/blob/main/docs/aisloader.md#writing-an-object-name-list) for which options apply.
When `listOut` is set, `config` cannot set `filelist` or `filelist-obj`.
To reuse an existing list without listing again, leave `listOut` unset and set `filelist-obj` to the list in `config`.
