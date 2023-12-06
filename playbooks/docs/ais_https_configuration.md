
# AIS with HTTPS

AIS supports HTTPS deployments in K8s, both via an initial HTTPS deployment or by converting an existing cluster. 

We provide playbooks in this repo for generating a self-signed certificate using [cert-manager](https://cert-manager.io/). If you have cert-manager configured to use a CA, you must ensure the output certs are stored in the same K8s secret the cluster is configured to use in `vars/https_config.yml`.

## Deploying with HTTPS

To deploy AIS with HTTPS initially: 
1. Edit the TLS variables in `vars/https_config.yml`
2. Create your TLS certificates
   - We provide a playbook for automating self-signed cert generation: [ais_generate_https_cert](ais_generate_https_cert.md)
3. Run the `ais_deploy_cluster` playbook to deploy as usual

## Switching an HTTP cluster to HTTPS

We also provide a playbook to transition from HTTP to HTTPS without losing any data in the cluster. 
First update the TLS variables and create a certficiate as described above. Next, follow the instructions in [ais_switch_protocol](ais_switch_protocol.md) to redeploy the cluster with access to that secret. 

## Connecting to the HTTPS cluster

For client connectivity after deploying with HTTPS, you have a few options.

1. Change the CLI config to skip X.509 verification:
   ```bash
   $ ais config cli set cluster.skip_verify_crt true
   ```
2. Get the issuer's ca.crt from the K8s secret, e.g. `kubectl get secret -n ais ais-root-secret -o jsonpath="{.data['ca\.crt']}" | base64 --decode > ca.crt`. Then copy the cert to your client machine and set the environment variable for the AIS CLI described in the [AIStore docs](https://github.com/NVIDIA/aistore/blob/master/docs/cli.md#environment-variables)
3. If using the Python SDK see the [SDK docs](https://github.com/NVIDIA/aistore/tree/master/python/aistore/sdk#readme)