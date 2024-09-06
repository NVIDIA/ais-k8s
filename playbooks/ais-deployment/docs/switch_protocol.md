# ais_switch_protocol

## Overview

The [`ais_restart_cluster.yml`](../ais_restart_cluster.yml) playbook serves the purpose of streamlining the transition between HTTP and HTTPS-based deployments for AIStore while preserving all data, including buckets and objects.

## Usage

### Prerequisites

Before running this playbook, ensure the following prerequisites are met:

1. **AIStore Cluster Configuration:** Verify that your AIStore cluster is properly configured and accessible via the command line interface (CLI).

2. **CLI Configuration:** Using the CLI, perform the following steps with the correct cluster endpoint set to `AIS_ENDPOINT`:

   - To disable HTTPS:
     ```bash
     ais config cluster net.http.use_https false
     ```

   - To enable HTTPS:
     ```bash
     ais config cluster net.http.use_https true
     ais config cluster net.http.skip_verify true
     ais config cluster net.http.server_key /var/certs/tls.key
     ais config cluster net.http.server_crt /var/certs/tls.crt
     ```

3. **Cluster Shutdown:** Gracefully shut down the cluster to ensure configurations are saved properly:
   ```bash
   ais cluster shutdown -y
   ```

   > **Note:** Shutting down the cluster ensures that the configuration changes are correctly saved and will be applied in subsequent runs. After shutting down, the cluster will be inaccessible until it is redeployed through the playbook.

4. **Certificate Creation and Mounting:** Follow [generate_https_cert](generate_https_cert.md) to create your TLS certificates.

   > **Note:** If you are using the AIS CLI and prefer not to verify the certificate, you can set `cluster.skip_verify_crt` to `true` with the command:  
   > ```bash
   > ais config cli set cluster.skip_verify_crt true
   > ```

### Playbook Execution

To execute the [`ais_restart_cluster.yml`](../ais_restart_cluster.yml) playbook, follow these steps:

1. **Install Ansible:** Ensure Ansible is installed on your system.

2. **Configure Hosts:** Create or update your `hosts.ini` file to specify the `controller` host and the `ais` hosts, which represent the nodes of your AIStore cluster.

3. **Update TLS Variables:** Modify the variables in `vars/https_config.yml` to reflect your TLS settings.

4. **Verify AIS Mountpaths:** Ensure that the mountpaths in `vars/ais_mpaths.yml` are accurate for your cluster.

5. **Run the Playbook:** Execute the playbook with the following command:
   ```bash
   ansible-playbook -i hosts.ini ais_restart_cluster.yml -e cluster=ais
   ```

   If you need to remove AIStore configuration files after significant upgrades, you can run:
   ```bash
   ansible-playbook -i hosts.ini ais_restart_cluster.yml -e cluster=ais -e delete_conf=true
   ```
