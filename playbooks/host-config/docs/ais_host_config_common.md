# ais_host_config_common

## Overview

This document provides a detailed guide for system tuning in the context of AIStore deployments. The configurations are adjustable to meet specific deployments. While some settings are __essential__ for AIStore, others are __recommended__ for enhancing performance, subject to site-specific adaptation.

## Usage

### Tagging Scheme

Different deployments, given their unique hardware and network environments, will necessitate different configurations.
To accommodate this, we have grouped configurations into categories applied with specific tags:

- `aisrequired`: Essential tuning for AIStore. Default OS settings might not be optimal, and some tweaking might be needed.
- `never`: These are site-specific configurations that should be reviewed and enabled explicitly. In Ansible, the "never" tag means these tasks are not selected by default and don't require explicit skipping.
- `nvidiastd`: (NVIDIA Standard) Common tasks we apply in our environment but are not universal.
- `io`: Configures the `aishostconfig` service to apply io tweaks set in the `blkdevtune` section of playbook variables.
- `ethtool`: Configures the `aishostconfig` service to apply ethtool tweaks in the `ethtool` section of playbook variables.
- `rmsvc`: Remove the `aishostconfig` service and its config file. 

The functional areas include:

| Area             | Tags   | Description |
|------------------|-------------------|-------------|
| `ulimits`        | `aisrequired`     | Adjusts `/etc/security/limits.conf` to set file descriptor limits as specified in this [file](https://raw.githubusercontent.com/NVIDIA/aistore/b732d063d837885474c1f801ed92e4c49754aef3/deploy/conf/limits.conf). |
| `mtu`            | `nvidiastd`, `mtu` | Sets MTU on Mellanox CX-5 NIC to 9000. |
| `cpufreq`        | `never`, `nvidiastd` | Sets the `performance` governor, ensuring necessary packages are installed. |
| `blkdevtune`| `io` | Configures an `aishostconfig` systemd service to set IO scheduler and queue settings.|
| `ethtool`| `ethtool` | Configures an `aishostconfig` systemd service to set ethtool settings.|

### Running the Playbook

1. Review and update the [playbook variables](../vars/host_config.yml) for your specific deployment. 
2. Reference the variables and [host config role](../roles/ais_host_config_common/tasks/main.yml) to see which tags to apply to your cluster. 
The `aisrequired` tag is a good place to start for minimal changes, but review the values carefully. 
3. Apply the host config with the tags selected, for example: 
```console
ansible-playbook -i hosts.ini ais_host_config_common.yml -e ais_hosts=ais -f 16 --tags aisrequired,mtu,io
```

Before executing the playbook, it's advisable to use `--list-tasks --list-tags` to verify the tasks that will be executed during the run.