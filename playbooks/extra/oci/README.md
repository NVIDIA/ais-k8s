## OCI Network Config Playbook

This playbook is used for our multi-home deployments in OCI. It is useful for configuring the OS on hosts to use a secondary VNIC as shown in the [OCI documentation](https://docs.oracle.com/iaas/compute-cloud-at-customer/topics/network/configuring-the-instance-os-for-a-secondary-vnic.htm#configuring-the-instance-os-for-a-secondary-vnic)

The script in [roles/configure_networks/files/oci_vnic_config.sh](./roles/configure_networks/files/oci_vnic_config.sh) is taken from the Oracle-provided link above and modified to work for our use case. 

By default, the provided script gave us issues with the network namespaces, so the script our role uses has been modified to work with our current Oracle Linux OKE instances. 

Specifically, we commented out the reading from network namespaces, as this created duplicate entries in the lists the script uses for configuring networks:

```bash
# for ns in "${nss[@]}"; do
#     oci_vcn_ip_ifaces_read $ns
# done
```

We also added an exception to filter out the `cni0` docker network, as this was not picked up as a virtual interface by default: 

```bash
if { [ -z "$IS_VM" ] || [ -z "${VIRTUAL_IFACES[$iface]}" ]; } && [ "${iface_data[1]}" != "cni0" ];
```
