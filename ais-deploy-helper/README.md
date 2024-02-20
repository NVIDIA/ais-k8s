## This directory contains scripts and templates for configuring or cleaning up host systems. 

### You should NOT need to edit anything in this folder to deploy -- it is only for building the helper docker images we use for running K8s jobs.

The resulting docker container, `ais-deploy-helper` is intended to contain all the files used by the ansible playbooks to prepare templates for `kubectl apply`. This allows us to script any extra configuration or cleanup into individual k8s jobs which can be applied via a template to the k8s cluster with no other host access required. 