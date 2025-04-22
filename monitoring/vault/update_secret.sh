#!/bin/bash

kubectl delete secret $LOCAL_ALLOY_SECRET -n monitoring --ignore-not-found

vault kv get -format json -field data $VAULT_ALLOY_SECRET | jq -r 'to_entries[] | "--from-literal=\(.key)=\(.value)"' | \
  xargs kubectl create secret generic -n monitoring $LOCAL_ALLOY_SECRET
