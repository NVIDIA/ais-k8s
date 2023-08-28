Playbooks in this directory are used for updating configurations related to remote cloud buckets, e.g. AWS s3. 

---
## AWS

To update AWS access for an AIStore cluster in k8s:

1. Add the desired AWS config and credentials files to `roles/files`
2. Run the aws playbook: `ansible-playbook -i inventory.yaml playbooks/cloud/ais_aws_config.yml`
3. This will copy the AWS files to the k8s controller host, create or overwrite the k8s secret, and remove the local files

Note: the AIStore cluster must already be configured with AWS support and an AWS kubernetes secret (see the [operator deployment instructions](../../operator/README.md) for more)