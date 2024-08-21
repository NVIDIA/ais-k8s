# AIStore Authentication Server (AuthN) in Kubernetes

The AIStore Authentication Server (AuthN) provides secure access to AIStore by leveraging [OAuth 2.0](https://oauth.net/2/) compliant [JSON Web Tokens (JWT)](https://datatracker.ietf.org/doc/html/rfc7519). 

For more information on AuthN, visit the [AIStore AuthN documentation](https://github.com/NVIDIA/aistore/blob/main/docs/authn.md).

## Setting Up AuthN in Kubernetes

To set up the AIStore Authentication Server (AuthN) in a production environment, follow these steps. All necessary specifications are provided in the [`authn.yaml`](../manifests/authn/authn.yaml) file. Please review and adjust the specifications and [environment variables](https://github.com/NVIDIA/aistore/blob/main/docs/authn.md#environment-and-configuration) as needed before applying the configurations.

### Using `kubectl`

You can apply the specifications using `kubectl`:

```bash
kubectl apply -f https://raw.githubusercontent.com/NVIDIA/ais-k8s/main/manifests/authn/authn-resources.yaml
kubectl apply -f https://raw.githubusercontent.com/NVIDIA/ais-k8s/main/manifests/authn/authn.yaml
kubectl apply -f https://raw.githubusercontent.com/NVIDIA/ais-k8s/main/manifests/authn/authn-services.yaml
```

#### Running AuthN with TLS

To enable TLS for the AuthN server, you'll need a certificate and a key. You can reuse the certificate and key generated for your AIStore deployment using the [generate certs playbook](../playbooks/ais-deployment/docs/generate_https_cert.md).

Once you have these, ensure they are stored in a Kubernetes secret named `tls-certs`. With this in place, you can deploy the AuthN server with TLS enabled by running:

```bash
kubectl apply -f https://raw.githubusercontent.com/NVIDIA/ais-k8s/main/manifests/authn/authn-resources.yaml
kubectl apply -f https://raw.githubusercontent.com/NVIDIA/ais-k8s/main/manifests/authn/authn-tls.yaml
kubectl apply -f https://raw.githubusercontent.com/NVIDIA/ais-k8s/main/manifests/authn/authn-services.yaml
```

This will configure the AuthN server to securely handle requests over HTTPS.

#### Deleting the AuthN Server and Its Resources

To completely remove the AuthN server along with all associated resources, execute the following command:

```bash
kubectl delete -f https://raw.githubusercontent.com/NVIDIA/ais-k8s/main/manifests/authn/authn-resources.yaml
kubectl delete -f https://raw.githubusercontent.com/NVIDIA/ais-k8s/main/manifests/authn/authn.yaml
kubectl delete -f https://raw.githubusercontent.com/NVIDIA/ais-k8s/main/manifests/authn/authn-services.yaml
```

This command ensures that the AuthN deployment, including the TLS configurations and any related resources, are fully deleted from your Kubernetes cluster.

### Using Ansible

Alternatively, you can use Ansible playbooks to [deploy](../playbooks/ais-deployment/ais_deploy_authn.yml) or [undeploy](../playbooks/ais-deployment/ais_undeploy_authn.yml) the AuthN server.

#### Deploy AuthN Server

```bash
ansible-playbook -i inventory.ini playbooks/ais-deployment/ais_deploy_authn.yml -e cluster=ais
```

#### Undeploy AuthN Server

To undeploy the AuthN server, run:

```bash
ansible-playbook -i inventory.ini playbooks/ais-deployment/ais_undeploy_authn.yml -e cluster=ais
```

### AuthN Resources in Kubernetes

1. **Signing Key Secret**  
   - This secret holds the key used to sign JWT tokens, which is used by both the AuthN server and AIStore pods.

2. **AuthN Configuration ConfigMap**  
   - The ConfigMap stores the default configuration of the AuthN server. Environment variables defined in specification of the AuthN deployment will override the config values at runtime.

3. **Persistent Storage (PV and PVC)**  
   - User information and configuration data for AuthN are securely stored in a Persistent Volume (PV), which is connected to the AuthN deployment via a Persistent Volume Claim (PVC).

4. **AuthN Deployment**  
   - This runs the AuthN container that provides the Authentication Server for AIStore.

5. **External Service for AuthN**  
   - This service exposes the AuthN server to external clients. You can choose to use either a `NodePort` or `LoadBalancer` service, depending on your access requirements.

6. **Internal Service for AuthN**  
   - This service facilitates internal communication between the AuthN server and other pods, including the AIS-Operator, within the cluster.

## How Components Interact with AuthN

When you enable authentication in an AIStore Cluster, all requests must include a valid JWT signed token. You can obtain a valid JWT token by logging in with the correct credentials on the AuthN server. AIStore verifies the signatures of these tokens using the secret created in **Step 1**. Requests without a token or with an invalid token are rejected. Here’s how different components interact with AuthN:

### AIS-Operator

If AuthN is enabled for your AIStore cluster, AIS-Operator requires a token since it frequently calls AIStore lifecycle APIs. AIS-Operator logs in as an admin user using the username and password specified in the [operator configuration](../operator/config/manager/manager.yaml). If your AuthN server is running at a different location, you can adjust the service host and port, along with the SU (admin) username and password, as shown below:

```yaml
- name: AIS_AUTHN_SU_NAME
  value: "admin" # Replace with the actual AuthN server admin username
- name: AIS_AUTHN_SU_PASS
  value: "admin" # Replace with the actual AuthN server admin password
- name: AIS_AUTHN_SERVICE_HOST
  value: "ais-authn.ais" # Replace with the actual AIS AuthN service host
- name: AIS_AUTHN_SERVICE_PORT
  value: "52001" # Replace with the actual AIS AuthN service port
- name: AIS_AUTHN_USE_HTTPS
  value: "true" # Set to "true" if AuthN is running with HTTPS
```

To apply these changes, edit the environment variables in the [spec file](../operator/config/manager/manager.yaml) and reapply it.

### AIStore Cluster

AIStore mainly verifies JWT tokens using the secret created in **Step 1**. Intra-cluster communication does not require tokens. AIStore does not call AuthN APIs; instead, AuthN calls AIStore APIs during cluster registration.

To add a signing key secret in AIStore, simply add the `authNSecretName` field to the AIStore CRD. The value of this field should be the secret created in **Step 1** that contains the secret signing key:

```yaml
authNSecretName: "jwt-signing-key"
```

### All Other Clients

To interact with AIStore, clients need a signed JWT token. By default, an `admin` user with super-user privileges is created with the password `admin`. You can change this password through [environment variables](https://github.com/NVIDIA/aistore/blob/main/docs/authn.md#environment-and-configuration). Admins can then create roles and assign users to those roles. For a typical setup process, refer to the [Getting Started Guide](https://github.com/NVIDIA/aistore/blob/main/docs/authn.md#getting-started).

Set the following environment variable to point to the appropriate AuthN server to log in and obtain the token:

```bash
# For external clients
export AIS_AUTHN_URL=http://<pods-ip>:30001

# For internal clients
export AIS_AUTHN_URL=http://ais-authn.ais:52001
```

## Switching Between HTTP and HTTPS (TLS) for the AuthN Server

To switch the protocol of an existing AuthN server from HTTP to HTTPS (or vice versa), you can apply the new configuration specification over the current deployment. This will automatically redeploy the AuthN server with the updated settings.

We strongly recommend using [Ansible Playbooks](#using-ansible) for this process. Ansible ensures all steps are handled consistently, including the additional configuration update required for the [AIS Operator Manager](#ais-operator). If you choose to apply the [specification](#using-kubectl) manually using `kubectl`, you’ll need to manually update the operator’s environment variables to ensure it communicates correctly with the reconfigured AuthN server.

To manually update the AIS Operator’s configuration, run:

```bash
kubectl edit deployment -n ais-operator-system ais-operator-controller-manager -o yaml
# Update the environment variables: admin username/password, use-https, port, and host as needed.
```

## Disabling AuthN in an Existing AIStore Deployment

If you have AuthN enabled but no longer wish to require authentication tokens for your requests or use AuthN features, you can easily disable it via the CLI or APIs/SDK with a simple configuration update:

```bash
ais config cluster set auth.enabled=false
```

In most cases, a restart of AIStore is not necessary for this change to take effect. However, if AIStore continues to request tokens with each API call, you may need to restart the servers for the configuration to apply properly.

## Enabling AuthN on a Running AIStore Server

> **Note:** Enabling AuthN on an already running AIStore server requires a cluster restart.

To enable AuthN, ensure that the JWT Signing Key Secret is created. Once the secret is set up, you’ll need to restart the cluster and clear the existing configurations on all nodes. This can be done using the [`ais_restart_cluster`](../playbooks/ais-deployment/ais_restart_cluster.yml) playbook with the `delete_conf=true` environment variable. This playbook will:

- Delete the AIStore CRD
- Shutdown the cluster
- Remove configuration mounts on the target nodes, allowing them to load new configs
- Redeploy AIStore using the [`ais_deploy_cluster`](../playbooks/ais-deployment/ais_deploy_cluster.yml) playbook

Be sure to specify the JWT signing key secret in the [defaults](../playbooks/ais-deployment/roles/ais_deploy_cluster/defaults/main.yml) file.

```bash
ansible-playbook -i hosts.ini ais_restart_cluster.yml -e cluster=ais -e delete_conf=true
```