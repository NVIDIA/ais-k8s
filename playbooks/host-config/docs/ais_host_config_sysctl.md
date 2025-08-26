# ais_host_config_sysctl

## Overview

This playbook is useful for tweaking hosts' [sysctl kernel config](https://man7.org/linux/man-pages/man5/sysctl.conf.5.html).


## Environment-specific Variables

We provide a [default vars file](../vars/host_config_sysctl.yml) as well as a [set of environment-specific overrides](../vars/environments/) that can be applied. 

Set the variable `env` to an existing environment in the `vars/environments` directory to override any default values with the contents of the `host_config_sysctl.yml` file in that environment. 
Note that variables are NOT merged, so each entry must be overridden in its entirety. 
You can set this with Ansible's `-e` option or with an environment variable e.g. `export ANSIBLE_EXTRA_VARS="env=<your-env>"`. 
See [ansible docs on variables](https://docs.ansible.com/ansible/latest/playbook_guide/playbooks_variables.html) for details. 


## Adding New Environments

1. Create a new directory: `vars/environments/your-environment/`.
2. Copy and modify an existing `host_config_sysctl.yml` to that environment directory. 


## Running the Playbook

Run the playbook with tags to apply specific sections of sysctl overrides from the [playbook variables](../vars/host_config_sysctl.yml) with the environment overrides.

Be sure to update the filename variables so that your values take precedence over existing files in `/etc/sysctl.d`. 
The convention is to begin the filename with a number, with higher numbers overriding previously applied configs. 

Example playbook run with an environment "production":

```console
ansible-playbook -i hosts.ini ais_host_config_sysctl.yml -e ais_hosts=ais -e env=production -f 16 --tags sysctlrequired
```

## Tagging Scheme

| Tag              | Description                                                                |
|------------------|----------------------------------------------------------------------------|--|
| `sysctlrequired` | Essential TCP tweaks .                                                     |
| `sysctlnet`      | Networking tuning for 100GigE environments, customizable as per your setup |
| `sysctlvm`       | Sysctls under `vm` such as memory management                               |