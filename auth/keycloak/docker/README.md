For AIS development it can be useful to run Keycloak outside of K8s. 
This doc includes some setup instructions for running Keycloak with the AIS realm so it can work with a local AIS deployment outside K8s. 

First, if using TLS, create a CSR based on the expected SANs. 
A config file is provided in this directory as a sample. 

`openssl req -new -nodes -out server.csr -newkey rsa:2048 -keyout server.key.pem -config openssl-san.cnf` 

Next use the CSR to generate a self-signed key and certificate.

`openssl x509 -req -in server.csr -signkey server.key.pem -out server.crt.pem -days 3650 -extensions req_ext -extfile openssl-san.cnf`

Now we can run the keycloak docker image with `start-dev`, provide our config, and automatically import our AIS realm.
Note this command expects to be run from this directory, modify as needed. 

```console
docker run --name keycloak \
   -v $(pwd)/../realm/aistore-realm.json:/opt/keycloak/data/import/aistore-realm.json \
   -v $(pwd)/server.crt.pem:/opt/keycloak/conf/server.crt.pem:ro \
   -v $(pwd)/server.key.pem:/opt/keycloak/conf/server.key.pem:ro \
   -e sample.env \
   -p 8443:8443 \
   quay.io/keycloak/keycloak:latest \
   start-dev --import-realm
```

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