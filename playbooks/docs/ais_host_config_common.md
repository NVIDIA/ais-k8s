# ais_host_config_common

## Purpose

Add host node packages (for debug and observability), perform system tuning,
etc. Because a number of these choices and tweaks are taste and
site specific, a tagging scheme allow selective application.

Only a few items are *required* for AIStore. Others are advised for
performance tuning, but likely require some site-specific review.

## Usage

### Tagging Scheme

It is clear that the full playbook we use to configure our hosts
(with their particular hardware, network environment etc) will not
apply to all deployments. You may also already have a set of
`sysctl` tuning suited to your environment. For this reason,
tasks are broken down into a few functional areas and additionally tagged with one
or more of thew following:
- `aisrequired` - required tuning for AIStore deployment; you may need more or less aggressive val;ues, but OS defaults very likely will not serve well
- `never` - site-specific tweaks that require review before explicitly enabling; the "never" tag is special in Ansible - it indicates tasks
that should not be selected by default, and so do not require explicit
skipping (i.e., we don't mean "don't ever apply this"!)
- `nvidiastd` - those "never" tasks that we always apply in our environment
- `aisdev` - those task that we apply only on development systems

The functional areas are:

Area | Additional tags | Description
---- | --------------- | -----------
`ulimits` | `aisrequired` |  Changes `/etc/security/limits.conf` to apply the soft and hard limit values for `nofiles` listed in `vars/host_config.yml` (1048576 default)
`sysctlrequired` | `aisrequired` | Applies the "required" sysctls described in `vars/host_config_sysctl.yml` `(net.core.somaxconn`, `net.ipv4.tcp_tw_reuse`, `net.ipv4.ip_local_port_range`, `net.ipv4.tcp_max_tw_buckets`)
`sysctlnetwork` | `never`, `nvidiastd` | Applies a set of networking tuning tweaks that work well for us on a 100GigE environment but which should just serve as a starting point for your environment (if you have not already optimized these). See comments in `vars/host_config_sysctl.yml`
`sysctlnetmisc` | `never`, `nvidiastd` | Applies some OS related sysctls listed in `vars/host_config_sysctl.yml` that you may want to review.
`mtu` | `never`, `nvidiastd` | Set the MTU on the Mellanox CX-5 NIC to 9000
`cpufreq` | `never`, `nvidiastd` | Selects the `performance` governor after making sure supporting packages are installed
`iosched_ethtool` | `never`, `nvidiastd` | Tune IO scheduler of HDD devices and some ethtool channels/rings settings; NOTE: the defaults in `vars/host_config.yml` select `mq-deadline` as IO scheduler, if MQ scheduling is not enabled change to `deadman` or enable it using the provided playbook.<br>Note that this playbook creates a new `systemd` service unit `aishostconfig` which uses a quick-n-dirty shell script to apply some `ethtool` and IO scheduler tuning. The IO scheduler tuning would be better implemented via the `udev` mechanism.
`debugpkgs` | `never`, `aisdev` | Removes `unattended-upgrades` and installs a slew of packages that are handy for debug but none of which are required for AIStore
`pcm` | `never`, `aisdev` | Installs the Intel PCM tools

### Vars files

The vars consumed by the playbooks are split out into a few files,
all with extensive comments. In most cases comments also list what
task tags the var has influence upon.

### Running the Playbook - Minimal

If just getting started and you're happy to tune some time after
deployment, then just run the required set after confirming
you're happy with the values in the `vars` files. Everything that
is not required is tagged as `never` so no further action is required to
suppress them:
```console
ansible-playbook -i hosts.ini ais_host_config_common.yml --become
```
As per the tags table above, that incantation will apply just
the `aisrequired` tasks.

Add additional tags as required using `--tags`

### Running the Playbook - Full

There's no avoiding it - you'll have to review at least the
vars files and ideally also the role tasks to understand
what they'll do to your OS install. Tweak vars values as needed,
and note which functional areas you may want to skip. Now knock
yourslef out with Ansible options as needed, e.g.,:
```console
ansible-playbook --forks 20 -i hosts.ini ais_host_config_common.yml --become \
 --tags never --skip-tags mtu
```

It's worth using `--list-tasks --list-tags` prior to the real run, to
confirm which tasks will run.