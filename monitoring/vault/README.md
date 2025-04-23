1. Follow the wiki page in the AIS Gitlab repo to get the appropriate DL access.
1. Copy vault.env from the Gitlab wiki
1. `export $(cat vault.env | xargs)`
1. `vault login -method=oidc role=storage-services`
1. `kubectl config use-context <your cluster>`
1. `./update_secret.sh`