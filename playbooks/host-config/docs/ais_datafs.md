
# ais_datafs_mkfs

## Purpose
This is a destructive playbook:
- first it unmounts any existing filesytem at the specified mountpoints on all nodes
- then it removes any entries for this mountpoint from `/etc/fstab` on all nodes
- then `mkfs.xfs` is run for all the devices on all nodes
- finally the new filesystem is mounted and an entry added to `/etc/fstab`

This playbook is intended for use in establishing an initial AIStore cluster, and
in adding additional nodes to an existing cluster. As with any `mkfs`, all existing
data on the requested devices will be lost. The playbook requires interactive
confirmation of your intentions (this can be disabled).

Clearly, this playbook is destructive in its actions - use with care!

## Usage

We don't define the `ais_hosts` variable in a vars file - it makes it
too easy to mistakenly `mkfs` all nodes of a an existing cluster when
adding a new node.

You can edit `ais_devices` in `vars/ais_datafs.yml` which makes sense
since the standrard expected config is to use the same devices on 
every node; or just include that list on the cmdline, too.

Usage (assuming `ais_devices` defined in `vars/ais_datafs.yml`)
```cat vars/ais_datafs.yml
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

$ ansible-playbook -i hosts.ini ais_datafs_mkfs.yml -e ais_hosts=ais
```

```console
$ ansible-playbook -i hosts.ini ais_datafs_mkfs.yml -e ais_hosts=ais
Are you sure you want to destroy and mkfs AIS filesystems on [ais], devices ['sda', 'sdb', 'sdc', 'sdd', 'sde', 'sdf', 'sdg', 'sdh', 'sdi', 'sdj']? Type 'yes' to confirm. [no]: yes
```

## ais_datafs_{mount,umount,umount_purge}

Some utility playbooks that aren't usually needed:

- `ais_datafs_mount` will mount filesystems if they're already in `/etc/fstab`
- `ais_datafs_umount` will umount filesystems
- `ais_datafs_umount_purge` will both umount and remove any `/etc/fstab` entries.

