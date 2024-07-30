# AIStore Authentication Server (AuthN) in Kubernetes

The AIStore Authentication Server (AuthN) provides secure access to AIStore by leveraging [OAuth 2.0](https://oauth.net/2/) compliant [JSON Web Tokens (JWT)](https://datatracker.ietf.org/doc/html/rfc7519). 

For more information on AuthN, visit the [AIStore AuthN documentation](https://github.com/NVIDIA/aistore/blob/main/docs/authn.md).

## Setting Up AuthN in Kubernetes

To set up the AIStore Authentication Server (AuthN) in a production environment, follow these steps. All necessary specifications are provided in the [`authn.yaml`](../manifests/authn/authn.yaml) file. Please review and adjust the specifications and [environment variables](https://github.com/NVIDIA/aistore/blob/main/docs/authn.md#environment-and-configuration) as needed before applying the configurations.

### Using `kubectl`

You can apply the specifications using `kubectl`:

```bash
kubectl apply -f manifests/authn/authn.yaml
```

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

### Steps to Run AuthN in Kubernetes

1. **Create a Secret for the Signing Key**  
   - This secret is used by both the AuthN server and the AIStore pods. It contains the key used to sign JWT tokens.

2. **Create a ConfigMap for AuthN Configuration**  
   - This ConfigMap contains the configuration template for the AuthN server. Environment variables in the Init container of the AuthN deployment will override this template.

3. **Deploy AuthN**  
   The deployment includes two containers:
   - **Init Container**: Replaces environment variables in the ConfigMap template.
   - **AuthN Container**: Uses the generated configuration with substituted variables and deploys the AuthN server.

4. **Deploy an External Service for AuthN**  
   This service allows access to the AuthN server from outside the cluster. You can use a `NodePort` service as shown in the example, or a `LoadBalancer` service.

5. **Deploy an Internal Service for AuthN**  
   This service allows other pods and the AIS-Operator to communicate with the AuthN server internally.

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
