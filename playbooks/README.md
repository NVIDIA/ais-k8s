This directory contains ansible playbooks for setting up an AIStore cluster in K8s

The playbooks are broken up into multiple sections, which should be executed in order. 

1. [host-config](./host-config/README.md) playbooks configure system settings on K8s nodes
2. [cloud](./cloud/README.md) playbooks set up credentials for accessing cloud backends, e.g. s3 and gcp
3. [ais-deployment](./ais-deployment/README.md) playbooks configure resources in the AIS namespace including the operator and the AIS cluster pods

An example hosts file is provided, [hosts-example.ini](./hosts-example.ini). You will need to set this up with your own hosts before running the playbooks.

Additional streamlined playbooks are also provided to apply recommended configurations to setup hosts by running the [`ais_host_config.yml`](ais_host_config.yml) playbook and deploy latest AIS Operator and AIS Cluster using the [`ais_deploy.yml`](ais_deploy.yml) playbook. 
```
$ # to configure the hosts for an AIS Deployment
$ ansible-playbook -i hosts.ini playbooks/ais_host_config.yml -e ais_hosts=ais

$ # to actually deploy AIS Cluster
$ ansible-playbook -i hosts.ini playbooks/ais_deploy.yml -e cluster=ais
```

For additional ansible config tweaks, you can create an `ansible.cfg` file. Check the [Ansible documentation](https://docs.ansible.com/ansible/latest/installation_guide/intro_configuration.html) for this, as options may change with new versions. 