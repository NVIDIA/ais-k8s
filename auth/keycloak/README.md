# AIStore Keycloak Integration

This directory contains instructions and reference installation for setting up an instance of [Keycloak](https://www.keycloak.org/) for AIStore authenticatication. 

See the [Keycloak quickstarts repo](https://github.com/keycloak/keycloak-quickstarts) for some other installation options.

Follow the [installation guide](./INSTALLATION.md) for instructions to get a sample deployment running on an existing cluster.

To create a non-production, automated deployment on a local KinD cluster, see [test-cluster.sh](./test-cluster.sh).

## AIStore Realm

The AIStore Realm is auto-imported in both the Docker and KinD deployment automation.

This realm comes by default with a client `AIStore` and a default admin role that can be assigned to users. 
Additional attributes can be added to match the JWT claim format described above. 

## Using test-cluster.sh

This provided script will set up a local KinD cluster along with a simple Keycloak deployment including all prerequisites and an AIStore realm. 

By default this deployment is set up for cluster-internal access.
You can use kubectl port-forward to expose the service on your local machine outside K8s: 

```bash
kubectl port-forward -n keycloak service/keycloak-server-service 8543:8543
```

Your request URL must match the hostname defined in [the keycloak manifest](./manifests/keycloak.yaml). 
Modify your etc/hosts file to route your request to the port on your local machine mapped above to the internal service.
For example: 

```bash
 cat /etc/hosts
127.0.0.1       localhost keycloak-server-service.keycloak.svc.cluster.local
```

Now you can get a token from the service running inside K8s on your machine:

```bash
curl -k \
  -d "client_id=AIStore" \
  -d "username=ais-admin" \
  -d "password=<your password>" \
  -d "grant_type=password" \
  "https://keycloak-server-service.keycloak.svc.cluster.local:8543/realms/aistore/protocol/openid-connect/token" | jq -r ".access_token"
```