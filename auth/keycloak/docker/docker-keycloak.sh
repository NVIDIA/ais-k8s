#!/usr/bin/env bash

docker run --rm --name keycloak \
   -v $(pwd)/db:/opt/keycloak/data/h2 \
   -v $(pwd)/../realm/aistore-realm.json:/opt/keycloak/data/import/aistore-realm.json \
   -v $(pwd)/server.crt.pem:/opt/keycloak/conf/server.crt.pem:ro \
   -v $(pwd)/server.key.pem:/opt/keycloak/conf/server.key.pem:ro \
   --env-file sample.env \
   -p 8443:8443 \
   quay.io/keycloak/keycloak:latest \
   start-dev --import-realm