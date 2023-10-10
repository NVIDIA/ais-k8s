# ais_switch_protocol

## Overview

The `ais_switch_protocol` playbook serves the purpose of streamlining the transition between HTTP and HTTPS-based deployments for AIStore while preserving all data, including buckets and objects.

## Usage

### Prerequisites

Before using this playbook, ensure you have the following prerequisites in place:

1. **AIStore Cluster Configuration:** Make sure that your AIStore cluster is properly configured and accessible via the command line interface (CLI).

2. **CLI Configuration:** Using the CLI, perform the following steps with the correct cluster endpoint set to `AIS_ENDPOINT`:

   - To disable HTTPS:
     ```bash
     $ ais config cluster net.http.use_https false
     ```

   - To enable HTTPS:
     ```bash
     $ ais config cluster net.http.use_https true
     $ ais config cluster net.http.skip_verify true
     ```

3. **Cluster Shutdown:** Safely shut down the cluster by running the following command:
   ```bash
   $ ais cluster shutdown
   ```

### Playbook Execution

Follow these steps to use the `ais_switch_protocol` playbook:

1. **Ansible Installation:** Ensure that Ansible is installed on your system.

2. **Host Configuration:** Create or edit your `hosts.ini` file to specify the `controller` host where you want to apply this playbook, as well as the `ais` hosts, which are the nodes of your AIStore cluster.

3. **Edit Defaults:** In the `main.yml` file located under `/playbooks/roles/ais_switch_protocol/defaults/main.yml`, specify the protocol to which you want to switch (HTTP or HTTPS).

4. **Run the Playbook:** Execute the playbook using the following command:
   ```console
   $ ansible-playbook -vvv -i hosts.ini ais_switch_protocol.yml -e cluster=ais --become -K
   ```

   This command will execute the playbook, seamlessly transitioning your deployment between HTTP and HTTPS while retaining your data intact.