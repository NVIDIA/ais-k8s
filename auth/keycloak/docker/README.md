# Keycloak In Docker

For AIS development it can be useful to run Keycloak outside of K8s. 
This doc includes some setup instructions for running Keycloak with the AIS realm so it can work with a local AIS deployment outside K8s. 

## TLS Config

First, if using TLS, create a CSR based on the expected SANs. 
A config file is provided in this directory as a sample. 

`openssl req -new -nodes -out server.csr -newkey rsa:2048 -keyout server.key.pem -config openssl-san.cnf` 

Next use the CSR to generate a self-signed key and certificate.

`openssl x509 -req -in server.csr -signkey server.key.pem -out server.crt.pem -days 3650 -extensions req_ext -extfile openssl-san.cnf`

## Running the Keycloak Container

Run [docker-keycloak.sh](./docker-keycloak.sh) to start the keycloak docker image, provide our config, and automatically import our AIS realm.
Note this command expects to be run from this directory, modify as needed. 

### Optional Data Persistence

If you want data persistence: 

- Use [recreate-volumes.sh](./recreate-volumes.sh)
- OR
  - Manually create a local `db` directory for data persistence. 
  - Give it access for the keycloak process in the docker container to write:  `sudo chown -R 1000:1000 $(pwd)/db`

This will mount keycloak's development server file-based database into a local `db` directory for data persistence between runs. 

### Run Container

Remove the volume mount for `db` if not allowing persistence.
Optionally use [docker-keycloak.sh](./docker-keycloak.sh) or manually run the command below.  

```bash
docker run --rm --name keycloak \
   -v $(pwd)/db:/opt/keycloak/data/h2 \
   -v $(pwd)/../realm/aistore-realm.json:/opt/keycloak/data/import/aistore-realm.json \
   -v $(pwd)/server.crt.pem:/opt/keycloak/conf/server.crt.pem:ro \
   -v $(pwd)/server.key.pem:/opt/keycloak/conf/server.key.pem:ro \
   --env-file sample.env \
   -p 8443:8443 \
   quay.io/keycloak/keycloak:latest \
   start-dev --import-realm
```

Check your connection  

```bash
curl --cacert server.crt.pem https://localhost:8443/realms/aistore
```

## Create an admin user

When first deployed, the AIStore realm does not include any users, which must exist to fetch tokens. 
You can create AIS users manually or use our utility Python script to create `ais-admin` (update args as needed): 

```bash
../scripts/prepare_cluster.sh https://localhost:8443 admin admin ./server.crt.pem
```

## Getting a Token

Once created, a token can be fetched from the above service with a curl command like the following:

```bash
curl -k \
  -d "client_id=AIStore" \
  -d "username=ais-admin" \
  -d "password=<your password>" \
  -d "grant_type=password" \
  "https://localhost:8443/realms/aistore/protocol/openid-connect/token" | jq -r ".access_token"
```

## Configuring AIStore

Starting with the v4.1 AIStore release, you can configure AIS to validate auth tokens via OIDC issuer lookup. 
A local AIS will be able to contact the Keycloak instance to validate the signature of any token issued by Keycloak. 
The config in AIS should look like this to support lookup: 

```json
"auth": {
   "enabled": true,
   "oidc":  {
      "allowed_iss": ["https://localhost:8443/realms/aistore"],
      "issuer_ca_bundle": "<your dir>/keycloak.crt.pem"
   }
}
```

If you don't need dynamic issuer lookup, you can also set the config for AIS with a static public key as shown below.
The public key can be fetched from keycloak 

```json
"auth": {
   "enabled": true,
   "signature": {
      "key": "<pubkey>",
      "method": "rsa",
   }
}
```

## Updating the AIStore Realm

For development purposes to update the realm: 
Run the above commands to start Keycloak with persistence. 
Modify the realm to the desired state, then shutdown. 

- Use [recreate-volumes.sh](./recreate-volumes.sh)
- OR
  - Manually create a local directory `exports` for the destination.
  - Give it access for the keycloak process in the docker container to write: `sudo chown -R 1000:1000 $(pwd)/exports`

Run this command to use the keycloak script to output the modified realm including users:

```bash 
docker run --rm \
  -v $(pwd)/db:/opt/keycloak/data/h2 \
  -v $(pwd)/exports:/opt/keycloak/data/export \
  quay.io/keycloak/keycloak:latest \
  export --dir /opt/keycloak/data/export --realm aistore --users realm_file
```