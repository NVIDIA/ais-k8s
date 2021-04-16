#!/bin/bash

apt-get -y update
apt-get install -y build-essential

curl -LO https://storage.googleapis.com/golang/go1.16.linux-amd64.tar.gz
tar -C /usr/local -xvzf go1.16.linux-amd64.tar.gz > /dev/null 2>&1
rm -rf go1.16.linux-amd64.tar.gz

git clone https://github.com/NVIDIA/aistore.git
cd aistore/cmd/cli/test && /usr/local/go/bin/go test -v .
