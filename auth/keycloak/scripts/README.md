This directory contains some utility scripts for setting up AIStore in a local development K8s cluster with Keycloak. 

Keycloak includes most realm settings in the realm export which can be imported to any deployment. 
For better compatibility and security, the scripts in here allow you to automatically create a new `ais-admin` user on startup. 