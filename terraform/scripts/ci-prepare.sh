#!/bin/bash

apt-get update -y
apt-get -qq install -y curl jq software-properties-common apt-transport-https ca-certificates gnupg psmisc wget

echo "Installing kubectl..."
curl -Lo /tmp/kubectl "https://storage.googleapis.com/kubernetes-release/release/$(curl -s https://storage.googleapis.com/kubernetes-release/release/stable.txt)/bin/linux/amd64/kubectl"
chmod +x /tmp/kubectl && mv /tmp/kubectl /usr/local/bin/kubectl

echo "Installing terraform..."
apt-get update && apt-get install -y gnupg software-properties-common
rm /usr/share/keyrings/hashicorp-archive-keyring.gpg
wget -O- https://apt.releases.hashicorp.com/gpg | \
gpg --dearmor | \
tee /usr/share/keyrings/hashicorp-archive-keyring.gpg
gpg --no-default-keyring \
--keyring /usr/share/keyrings/hashicorp-archive-keyring.gpg \
--fingerprint
echo "deb [signed-by=/usr/share/keyrings/hashicorp-archive-keyring.gpg] https://apt.releases.hashicorp.com jammy main" | \
tee /etc/apt/sources.list.d/hashicorp.list
apt-get update -y && apt-get install terraform -y
terraform -v

echo "Installing gcloud..."
echo "deb [signed-by=/usr/share/keyrings/cloud.google.gpg] http://packages.cloud.google.com/apt cloud-sdk main" | tee -a /etc/apt/sources.list.d/google-cloud-sdk.list
curl https://packages.cloud.google.com/apt/doc/apt-key.gpg | apt-key --keyring /usr/share/keyrings/cloud.google.gpg add -
apt-get update -y && apt-get -qq install -y google-cloud-sdk

echo "Installing helm..."
curl https://baltocdn.com/helm/signing.asc | apt-key add -
echo "deb https://baltocdn.com/helm/stable/debian/ all main" | tee /etc/apt/sources.list.d/helm-stable-debian.list
apt-get update -y && apt-get -qq install -y helm