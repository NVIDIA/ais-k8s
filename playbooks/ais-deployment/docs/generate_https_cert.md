# Generate self-signed cert with generate_https_cert

## Purpose

The `generate_https_cert` playbook bootstraps a CA issuer and uses it to issue certificates for the AIS cluster, stored securely as a kubernetes secret.

## Usage

To use this playbook, follow these steps:

1. Make sure you have Ansible installed on your system.

2. Create or edit your `hosts.ini` file to specify the `controller` host where you want to apply this playbook.

3. Update the variables to set namespace, DNS, and secret names in [vars/https_config.yml](../vars/https_config.yml)

4. Run the playbook using the following command:

   ```console
   $ ansible-playbook -i hosts.ini generate_https_cert.yml
   ```
   This will execute the playbook and create the self-signed certificate on the specified controller host.

   To optionally output the resulting CA certificate to a local file, provide the `cacert_file` variable:

   ```console
   $ ansible-playbook -i hosts.ini generate_https_cert.yml -e cacert_file=local_ais_ca.crt -e cluster=ais
   ```

   To fetch the certificate later, you can [use the fetch_ca_cert playbook](./ais_https_configuration.md#fetching-ca-certificate)
