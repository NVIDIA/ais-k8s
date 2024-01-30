#!/bin/bash

multus_tar=tmp_multus_install.tar.gz 
multus_dir=tmp_multus_install
# Download the multus source
curl -L -o $multus_tar $1
mkdir -p $multus_dir && tar -xzf $multus_tar -C $multus_dir --strip-components=1
# Apply the daemonset
kubectl apply -f ./$multus_dir/deployments/multus-daemonset-thick.yml
# Cleanup
rm $multus_tar
rm -r $multus_dir
