# ais_https_cert

## Purpose

The `ais_https_cert` playbook automates the creation of a self-signed certificate using cert-manager. This certificate is then stored securely as a secret named `ais-tls-cert`.

## Usage

To use this playbook, follow these steps:

1. Make sure you have Ansible installed on your system.

2. Create or edit your `hosts.ini` file to specify the `controller` host where you want to apply this playbook.

3. Run the playbook using the following command:

   ```console
   $ ansible-playbook -i hosts.ini ais_https_cert.yaml
   ```

   This will execute the playbook and create the self-signed certificate on the specified controller host.
