# AIStore Authentication Server (AuthN) in Kubernetes

>  **NOTE**: AuthN and its related deployment automations are under development. Breaking changes are to be expected, and it has NOT gone through a complete security audit.
Please review your deployment carefully and follow [our security policy](https://github.com/NVIDIA/ais-k8s/blob/main/SECURITY.md) to report any issues.

The AIStore Authentication Server (AuthN) provides secure access to AIStore by leveraging [OAuth 2.0](https://oauth.net/2/) compliant [JSON Web Tokens (JWT)](https://datatracker.ietf.org/doc/html/rfc7519). 

For more information on AuthN, visit the [AIStore AuthN documentation](https://github.com/NVIDIA/aistore/blob/main/docs/authn.md).

## Setting Up AuthN in Kubernetes

### Deploy with Helm

The best way to deploy authN is to use our [provided Helm chart](../helm/authn/README.md)

### Deploy with Ansible

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

- **Static Resources**
  - **Signing Key Secret**  
     - This secret holds the key used to sign JWT tokens, which is used by both the AuthN server and AIStore pods.
  - **Admin Credentials Secret**
     - This secret contains the admin user and password as entries, mapped to `SU-NAME` and `SU-PASS`.
  - **AuthN Configuration ConfigMap**  
     - The ConfigMap stores the non-sensitive default configuration of the AuthN server.
  - **Persistent Storage (PV and PVC)**  
     - User information and configuration data for AuthN are stored in a Persistent Volume (PV), which is connected to the AuthN deployment via a Persistent Volume Claim (PVC).
- **Services**
  - **External Service for AuthN**
    - This service exposes the AuthN server to external clients. You can choose to use either a `NodePort` or `LoadBalancer` service, depending on your access requirements.
  - **Internal Service for AuthN**
     - This service facilitates internal communication between the AuthN server and other pods, including the AIS-Operator, within the cluster.
- **AuthN Deployment**  
   - This runs the AuthN pod and connects it with the other resources.
- **Operator AuthN ConfigMap**
  - To enable communication between the AIS K8s Operator and an AuthN-enabled AIS cluster, the operator must have access to AuthN server details and credentials.
  - [See the ConfigMap Helm Chart Docs](../helm/operator/authn-cm/README.md) for more details on creating this ConfigMap.

## How Components Interact with AuthN

When you enable authentication in an AIStore Cluster, all requests must include a valid signed JWT token.
You can obtain a token by logging in with the correct credentials on the AuthN server.
AIStore verifies the signatures of these tokens with the JWT signing key mounted from the secret created by AuthN.
Requests without a token or with an invalid token are rejected. 

Here’s how different components interact with AuthN:

### AIS Operator

If AuthN is enabled for your AIStore cluster, AIS Operator requires a token since it frequently calls AIStore lifecycle APIs. 

AIS Operator logs in as an admin user using the username and password specified for each cluster in the ConfigMap defined by `AIS_AUTHN_CM`.
By default, the operator will look for the ConfigMap `ais-operator-authn`. 
This is defined in the manifest when deploying the operator (see [here](../operator/config/overlays/default/manager_env_patch.yaml) for the kustomize overlay).

The operator will look up the value by the cluster's `namespace`-`name` to find which secret it should load for admin credentials as well as config information for the AuthN service.
It will use that config to fetch a token to use for admin access to the AIS cluster API.

If using Helm, [we provide a chart](../helm/operator/authn-cm/README.md) to manage this ConfigMap.

### AIStore Cluster

AIStore verifies JWT tokens using the AuthN signing key secret created at deployment.
Intra-cluster communication does not require tokens.
AIStore does not call AuthN APIs; instead, AuthN calls AIStore APIs during cluster registration.

To add a signing key secret to an AIStore cluster, simply add the `authNSecretName` field to the AIStore CRD.

```yaml
authNSecretName: "jwt-signing-key"
```

### All Other Clients

To interact with AIStore, clients need a signed JWT token.
By default, an `admin` user with super-user privileges is created with a mandatory provided password.
This password must be set through [environment variables](https://github.com/NVIDIA/aistore/blob/main/docs/authn.md#environment-and-configuration).
Admins can then create roles and assign users to those roles.
For a typical setup process, refer to the [Getting Started Guide](https://github.com/NVIDIA/aistore/blob/main/docs/authn.md#getting-started).

Set the following environment variable to point to the appropriate AuthN server to log in and obtain the token:

```bash
# For external clients
export AIS_AUTHN_URL=http://<NodePort-service-IP>:30001

# For internal clients
export AIS_AUTHN_URL=http://ais-authn.ais:52001
```

## Switching Between HTTP and HTTPS (TLS) for the AuthN Server

To switch the protocol of an existing AuthN server from HTTP to HTTPS (or vice versa), you can apply the new configuration specification over the current deployment.
This will automatically redeploy the AuthN server with the updated settings.

We strongly recommend using the [AuthN Helm chart](../helm/authn/README.md) for this process.

This will also require an update to the ConfigMap used for the operator. 
See [AIS Operator section above](#ais-operator)

## Disabling AuthN in an Existing AIStore Deployment

If you have AuthN enabled but no longer wish to require authentication tokens for your requests or use AuthN features, you can easily disable it via the CLI or APIs/SDK with a simple configuration update:

```bash
ais config cluster set auth.enabled=false
```

In most cases, a restart of AIStore is not necessary for this change to take effect. 
However, if AIStore continues to request tokens with each API call, you may need to restart the servers for the configuration to apply properly.

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