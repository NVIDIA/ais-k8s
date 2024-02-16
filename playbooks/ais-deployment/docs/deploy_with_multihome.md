# Multihome Deployment

To take advantage of multiple network interfaces, AIS supports multi-homing to distribute traffic across all avaialable interfaces. The operator currently supports using [multus](https://github.com/k8snetworkplumbingwg/multus-cni) to enable multiple IPs for each K8s pod.

`Note: Using more than 2 network interfaces has not yet been tested. Please reference the multus documentation or raise an issue if you run into errors assigning multiple IPs to a pod.`

### Host Prequisites

Before updating the K8s cluster or deploying AIS, the hosts must be configured with each network interface having its own IP accessible by each of the other nodes in the K8s cluster. All nodes must be able to connect to all interfaces on all other nodes. 

### Cluster Preparation

By default, each K8s pod has access to only one IP on the default interface. To enable multiple interfaces, we'll need to install and use [multus](https://github.com/k8snetworkplumbingwg/multus-cni). This allows us to define additional networks for each pod to use. To install, follow the instruction on the multus github page or simply run the network attachment definition playbook as shown below to automatically install the latest release. 

### Creating NetworkAttachmentDefinition

The next step is to define a network attachment definition to specify how each pod can use additional interfaces. This can be configured differently for each deployment, but we provide a sample template for a simple macvlan bridge definition [here](../roles/create_network_definition/files/nad.template.yaml).

For more info on creating your own definitions, check the [multus documentation](https://github.com/k8snetworkplumbingwg/multus-cni/blob/master/docs/how-to-use.md#create-network-attachment-definition).

To create network definitions with the default macvlan bridge, first update the [multihome variables](../vars/multihome.yml):

```yaml
# Any name for your network attachment (can be comma-separated list)
network_attachment: "macvlan-conf"

# Name of the interface for which to create a network attachment definition
# This can also be a comma-separated list, and must match the length of the network_attachment list as each entry will be paired together
network_interface: "ens7f1np1"

# Namespace for the attachment (this should match your AIS cluster namespace)
attachment_namespace: "ais"
```

Next, run the `create_network_definition` playbook:

`ansible-playbook -i ../hosts.ini create_network_definition.yml`

This will create one or more network attachment definitions as custom resource definitions in your K8s cluster. You can check existing definitions with kubectl:

`kubectl get network-attachment-definitions -n ais`

### Deploying with Defined Hosts

Finally, we can deploy AIS and use the definitions we created above. 

1. Update each host entry your ansible inventory file with the additional_hosts variable. This can be a comma-separated list for multiple additional IPs.

    `additional_hosts=IP2,IP3,etc.` e.g.

    ```ini
    worker1 ansible_host=10.51.248.68 additional_hosts=10.51.248.116
    worker2 ansible_host=10.51.248.77 additional_hosts=10.51.248.105
    ```

2. Make sure `network_attachment` is properly set in [multihome variables](../vars/multihome.yml). This can also be a comma-separated list if you've created multiple definitions in your K8s cluster. 

3. Run the deploy playbook as usual:

    `ansible-playbook -i ../hosts.ini ais_deploy_cluster.yml -e cluster=ais -e node_name=ansible_host`

4. Check the cluster map.

    The resulting cluster can be checked with `ais show cluster smap --json`. The result should look something like the snippet below for each pod, with all additional hosts showing up in the `pub_extra` entry. 

    ```json
    "public_net": {
        "node_ip_addr": "10.51.248.68",
        "daemon_port": "51081",
        "direct_url": "http://10.51.248.68:51081"
    },
    "pub_extra": [
        {
            "node_ip_addr": "10.51.248.116",
            "daemon_port": "51081",
            "direct_url": "http://10.51.248.116:51081"
        }
    ],
    ```

### Troubleshooting

If you see errors with the pods initializing or are missing the `pub_extra` field in the cluster map, check the following:

- Make sure your ais-operator has been deployed with at least version v0.97 and your aisnode version is at least v3.22.
- Check that your network attachment definitions exist in the proper namespace:

    `kubectl get network-attachment-definitions -n <your namespace>`

- Check the populate-env container logs. This container runs before the aisnode image and creates environment variables based on the cluster configuration. It maps the primary host IP to those passed by the `additional_hosts` variables to create the `AIS_PUBLIC_HOSTNAME` environment variable for AIS to use. You can check the logs with the following command: 

    `kubectl logs -n ais <failing ais pod> -c populate-env`
    
    If all is correct, you should see something like `Setting AIS_PUBLIC_HOSTNAME to value from configMap: 10.51.248.77,10.51.248.105` with a list of all IPs for the pod. 
