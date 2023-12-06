# Generate self-signed cert with ais_https_cert

## Purpose

The `ais_https_cert` playbook bootstraps a CA issuer and uses it to issue certificates for the AIS cluster, stored securely as a kubernetes secret.

## Usage

To use this playbook, follow these steps:

1. Make sure you have Ansible installed on your system.

2. Create or edit your `hosts.ini` file to specify the `controller` host where you want to apply this playbook.

3. Update the variables to set namespace, DNS, and secret names in `vars/https_config.yml`

4. Run the playbook using the following command:

   ```console
   $ ansible-playbook -i hosts.ini ais_https_cert.yml
   ```

   This will execute the playbook and create the self-signed certificate on the specified controller host.
