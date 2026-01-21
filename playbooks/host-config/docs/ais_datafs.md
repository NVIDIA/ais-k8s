
# ais_datafs_mkfs

**This playbook is destructive - use with caution!**

## Purpose

This playbook is intended for establishing an initial AIStore cluster and adding additional nodes to an existing cluster.

`ais_datafs_mkfs` performs the following actions on all nodes:
- Unmounts any existing filesytem at the specified mountpoints
- Removes any entries for this mountpoint from `/etc/fstab`
- Creates an `xfs` filesystem for all devices
- Mounts filesystems and adds entries to `/etc/fstab`


**All existing data on the requested devices will be lost.**
The playbook requires interactive confirmation of your intentions.

## Usage

### Variables

Two variables must be set to run this playbook

- `ais_hosts`
- `ais_devices`

Both can be provided as an [Ansible playbook variable](https://docs.ansible.com/projects/ansible/latest/playbook_guide/playbooks_variables.html).

`ais_devices` can also be set directly in `vars/ais_datafs.yml`.
The standard expected config is to use the same devices on every node.

We don't define the `ais_hosts` variable in a vars file - it makes it too easy to mistakenly `mkfs` all nodes of a an existing cluster when adding a new node.
Instead, assign `ais_hosts` at runtime to reference an Ansible inventory group (see example below). 

Usage (assuming `ais_devices` defined in `vars/ais_datafs.yml`):
```console
cat vars/ais_datafs.yml
ais_devices:
  - sda
  - sdb
  - sdc
  - sdd
  - sde
  - sdf
  - sdg
  - sdh
  - sdi
  - sdj

ansible-playbook -i hosts.ini ais_datafs_mkfs.yml -e ais_hosts=ais
```

If you want to skip prompts for verification of disk unmounting and formatting: 
```console
ansible-playbook -i hosts.ini ais_datafs_mkfs.yml -e ais_hosts=ais -e check_mounts=false -e prompt_for_format=false
```


## ais_datafs_{mount,umount,umount_purge}

Some utility playbooks that aren't usually needed:

- `ais_datafs_mount` will mount filesystems that are already created
- `ais_datafs_umount` will umount filesystems.
- `ais_datafs_umount_purge` will both umount and remove any `/etc/fstab` entries.

