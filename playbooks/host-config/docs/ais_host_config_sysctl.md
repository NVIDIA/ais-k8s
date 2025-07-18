# ais_host_config_sysctl

## Overview

This playbook is useful for tweaking hosts' [sysctl kernel config](https://man7.org/linux/man-pages/man5/sysctl.conf.5.html).

## Usage

Run the playbook with tags to apply specific sections of sysctl overrides from the [playbook variables](../vars/host_config_sysctl.yml).

Be sure to update the filename variables so that your values take precedence over existing files in `/etc/sysctl.d`. 
The convention is to begin the filename with a number, with higher numbers overriding previously applied configs. 

Example playbook run:

```console
ansible-playbook -i hosts.ini ais_host_config_sysctl.yml -e ais_hosts=ais -f 16 --tags sysctlrequired
```

### Tagging Scheme

| Tag              | Description                                                                |
|------------------|----------------------------------------------------------------------------|--|
| `sysctlrequired` | Essential TCP tweaks .                                                     |
| `sysctlnet`  | Networking tuning for 100GigE environments, customizable as per your setup |
| `sysctlvm`     | Sysctls under `vm` such as memory management                                    |