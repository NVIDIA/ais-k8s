# AIS Admin Client Pod

This chart can be used to deploy a pod with the [ais-util container](https://hub.docker.com/r/aistorage/ais-util) to a K8s cluster. 

This util container contains binaries for our client tools: 
- AIS CLI
- AIS Python SDK
- AISloader

The [values.yaml](./values.yaml) can be modified to deploy any docker image and configure other details about the deployment such as node selection. 

Run `helmfile sync` from this directory to apply (or helm install manually).

## Using the Client

Run this command to open a shell in the pod: 

```bash
kubectl exec -n ais -it deploy/ais-client -- /bin/bash
```

The environment will be pre-configured to use AIS CLI commands.

## TLS Configuration

### Without TLS

If your AIS cluster does not use TLS, omit `tls.caConfigMap` and use `http://` in `ais.endpoint`.

### With TLS Certificate Verification

If you have an AIS cluster with TLS enabled, you can use [trust-manager](https://cert-manager.io/docs/trust/trust-manager/) to load a self-signed certificate into this client pod for trust. 

A simple manifest is provided to create a trust bundle for a self-signed cluster issuer: [trust-bundle.yaml](trust-bundle.yaml)

This will create a ConfigMap `aistore.nvidia.com` in the AIS namespace which is included in the helm values as a mount for the client to trust.

```yaml
tls:
  insecureSkipVerify: false
  caConfigMap: aistore.nvidia.com
  bundleFile: trust-bundle.pem
```

To skip certificate verification, set `tls.insecureSkipVerify: true`.

## AuthN Configuration

To enable authentication, set `authn.enabled: true`:

```yaml
authn:
  enabled: true
  serviceURL: https://ais-authn.ais:52001
  secretName: ais-authn-su-creds
```

When AuthN is enabled, the following environment variables are set in the client pod:

- `AIS_AUTHN_URL`: The authentication service URL
- `AIS_AUTHN_USERNAME`: The username (from secret if `authn.secretName` is set)
- `AIS_AUTHN_PASSWORD`: The password (from secret if `authn.secretName` is set)

From the client, login using the environment variables from the secret as follows:

```bash
ais auth login "$AIS_AUTHN_USERNAME" -p "$AIS_AUTHN_PASSWORD"
```

> **Note:** If `authn.secretName` is omitted, you can use external secret injection (e.g. Vault) to provide credentials directly.
