# ais_host_config_common

## Overview

This document provides a detailed guide for system tuning in the context of AIStore deployments. The configurations are adjustable to meet specific deployments. While some settings are __essential__ for AIStore, others are __recommended__ for enhancing performance, subject to site-specific adaptation.

## Usage

### Tagging Scheme

Different deployments, given their unique hardware and network environments, will necessitate different configurations. To accommodate this, we have categorized tasks into several functional areas, each marked with one or more of the following tags:

- `aisrequired`: Essential tuning for AIStore. Default OS settings might not be optimal, and some tweaking might be needed.
- `never`: These are site-specific configurations that should be reviewed and enabled explicitly. In Ansible, the "never" tag means these tasks are not selected by default and don't require explicit skipping.
- `nvidiastd`: (NVIDIA Standard) Common tasks we apply in our environment but are not universal.
- `aisdev`: Tasks specifically for development systems.

The functional areas include:

| Area             | Additional Tags   | Description |
|------------------|-------------------|-------------|
| `ulimits`        | `aisrequired`     | Adjusts `/etc/security/limits.conf` to set file descriptor limits as specified in this [file](https://raw.githubusercontent.com/NVIDIA/aistore/b732d063d837885474c1f801ed92e4c49754aef3/deploy/conf/limits.conf). |
| `sysctlrequired` | `aisrequired`     | Implements essential sysctls from `vars/host_config_sysctl.yml`. |
| `sysctlnetwork`  | `never`, `nvidiastd` | Networking tuning for 100GigE environments, customizable as per your setup. See `vars/host_config_sysctl.yml` for details. |
| `sysctlnetmisc`  | `never`, `nvidiastd` | OS-related sysctls for review, listed in `vars/host_config_sysctl.yml`. |
| `mtu`            | `never`, `nvidiastd` | Sets MTU on Mellanox CX-5 NIC to 9000. |
| `cpufreq`        | `never`, `nvidiastd` | Sets the `performance` governor, ensuring necessary packages are installed. |
| `iosched_ethtool`| `never`, `nvidiastd` | Configures IO scheduler and ethtool settings. Defaults to `mq-deadline` scheduler; use `deadman` if MQ scheduling is not enabled. Includes a `systemd` service for applying these settings. |

### Configuration Variables

The playbooks rely on various variables, detailed in separate files with extensive comments. These comments also indicate which tasks are influenced by each variable.

### Running the Playbook

#### Basic Setup

For initial setups or when post-deployment tuning is planned:

1. Review and agree with the values in the `vars` files.
2. Run only the essential tasks (tagged `aisrequired`) to avoid unnecessary configurations:

   ```console
   ansible-playbook -i hosts.ini ais_host_config_common.yml --become
   ```

### Running the Playbook - Full

It's essential to thoroughly examine the variable files and, if possible, the role tasks to fully grasp their impact on your operating system setup. Modify the variable values as required, and identify any functional areas you may prefer to exclude. Feel free to utilize various Ansible options to tailor your execution. For example:
```console
ansible-playbook --forks 20 -i hosts.ini ais_host_config_common.yml --become \
 --tags never --skip-tags mtu
```

Before executing the playbook, it's advisable to use `--list-tasks --list-tags` to verify the tasks that will be executed during the run.