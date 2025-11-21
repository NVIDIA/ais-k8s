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

Now we can run the keycloak docker image with `start-dev`, provide our config, and automatically import our AIS realm.
Note this command expects to be run from this directory, modify as needed. 

```bash
docker run --rm --name keycloak \
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

## Getting a Token

As of now, the AIStore realm does not include any users, which must exist to fetch tokens. 
Once created, a token can be fetched from the above service with a curl command like the following:

```bash
curl -k \
  -d "client_id=AIStore" \
  -d "username=ais-admin" \
  -d "password=password" \
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