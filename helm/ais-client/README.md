# AIS Admin Client Pod

This chart can be used to deploy a pod with the [ais-util container](https://hub.docker.com/r/aistorage/ais-util) to a K8s cluster. 

This util container contains binaries for our client tools: 
- AIS CLI
- AIS Python SDK
- AISloader

The [values.yaml](./values.yaml) can be modified to deploy any docker image and configure other details about the deployment such as node selection. 

Run `helmfile sync` from this directory to apply (or helm install manually).

## Running without CA Trust

If your AIS cluster does NOT use TLS or if you don't need a mounted CA to trust, you'll need to run `unset AIS_CLIENT_CA` before using the AIS tools.

This prevents them from trying to load the non-existent CA trust bundle inside the pod. 

## Trust Manager Config

If you have an AIS cluster with TLS enabled, you can use [trust-manager](https://cert-manager.io/docs/trust/trust-manager/) to easily load a self-signed certificate into this client pod for trust. 

A simple manifest is provided to create a trust bundle for a self-signed cluster issuer: [trust-bundle.yaml](trust-bundle.yaml)

This will create a ConfigMap `aistore.nvidia.com` in the AIS namespace which is included in the helm values as a mount for the client to trust. 

## Using the pod

Run this command to open a shell in the pod: 

`kubectl exec -n ais -it deploy/ais-client -- /bin/bash`

The environment will be pre-configured to use AIS CLI commands with certificate trust. 