
# AIS with HTTPS

AIS supports HTTPS deployments in K8s, both via an initial HTTPS deployment or by converting an existing cluster. 

We provide playbooks in this repo for generating a self-signed certificate using [cert-manager](https://cert-manager.io/). If you have cert-manager configured to use a CA, you must ensure the output certs are stored in the same K8s secret the cluster is configured to use in `vars/https_config.yml`.

## Deploying with HTTPS

To deploy AIS with HTTPS initially: 
1. Edit the TLS variables in [vars/https_config.yml](../vars/https_config.yml)
2. Create your TLS certificates
   - We provide a playbook for automating self-signed cert generation: [generate_https_cert](generate_https_cert.md)
3. Run the `ais_deploy_cluster` playbook to deploy as usual

## Switching an HTTP cluster to HTTPS

We also provide a playbook to transition from HTTP to HTTPS without losing any data in the cluster. 
First update the TLS variables and create a certficiate as described above. Next, follow the instructions in [switch_protocol](switch_protocol.md) to redeploy the cluster with access to that secret. 

## Connecting to the HTTPS cluster

For client connectivity after deploying with HTTPS, you have a few options.

1. Change the CLI config to **skip X.509 verification**:
   ```bash
   $ ais config cli set cluster.skip_verify_crt true
   ```

2. Get the issuer's ca.crt from the K8s secret using the playbook described [below](#fetching-ca-certificate) 

3. Set up your client to use the certificate for verification
   - **CLI**:  Set the environment variable (`AIS_CLIENT_CA`) for the AIS CLI described in the [AIStore docs](https://github.com/NVIDIA/aistore/blob/master/docs/cli.md#environment-variables)
   - **Python SDK** See the [SDK docs](https://github.com/NVIDIA/aistore/tree/master/python/aistore/sdk#readme)
   - **HTTP (curl)** Use the `cacert` option, e.g. `curl https://localhost:51080/v1/daemon?what=smap --cacert ca.crt`

### Fetching CA certificate

To fetch the CA certificate for verifying the server's cert on the client side, you can use the `fetch_ca_cert` playbook. Provide the `cacert_file` argument to specify the output file (default is `ais_ca.crt`). This will fetch the certificate from the K8s secret on the cluster so it can be used with a local client. 

```
ansible-playbook -i ../../hosts.ini fetch_ca_cert.yml -e cacert_file=ca.crt -e cluster=ais
```

> Note: The crt file is stored under `ais-k8s/playbooks/ais-deployment`

To use this `ais_ca.crt` with CLI, run - `export AIS_CLIENT_CA=<path-to-cert>/ais_ca.crt`
