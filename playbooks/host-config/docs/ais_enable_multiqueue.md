# ais_enable_multiqueue

## Purpose

Choosing `mq-deadline` over `deadline` offers a small performance win.
Multiqueue IO schedulers are not enabled by default in Ubuntu 18.04 - they become
the default in later versions of the Linux kernel.

If your host install process does not already enable MQ then you can use this
playbook to enable MQ; the playbook changes Grub config, and requires a
reboot for effect. You can see whether `mq-deadline` is available using
`cat /sys/block/sda/queue/scheduler` (substituting `sda` for your devices) - if it does not appear in the available list then consider enabling it.

Note that this playbook simply enables MQ IO scheduling - the selection
of `mq-deadline` is performed in `ais_host_config_common.yml`.

## Usage

```console
$ ansible-playbook -i hosts.ini ais_enble_multiqueue.yml -e ais_hosts=ais
```

We need only apply this against nodes that will host AIStore target 
nodes.

The playbook notes that a reboot is required but does not initiate reboot.
