# AIS Host Configuration — Cloud Init Scripts

Standalone bash scripts that replicate the host-level configuration from the
[Ansible playbooks](../../playbooks/host-config/), built for use in cloud environments.

These scripts configure the **host OS only** — they do not deploy AIStore itself.

## Platform Notes

The scripts use standard Linux interfaces (udev, sysctl, sysfs, fstab)
and should work on any distribution that provides the dependencies listed above.
The Ansible playbooks the scripts are based on have been tested with Ubuntu and RHEL.

## Scripts

| Script | Purpose | Standalone |
|--------|---------|------------|
| `ais_host_config.sh` | Main entry point — runs all host tuning and calls `ais_datafs.sh`|
| `ais_datafs.sh` | Creates filesystems and mounts block devices for AIS data paths |
| `install_deps.sh` | Installs missing packages and `yq` (auto-detects `apt-get` / `dnf` / `yum`) |
| `config.yaml.example` | Example YAML config file — copy and edit for your environment |
| `aws-eks-userdata.sh.example` | Sample launch template userdata for AWS EKS managed node groups |

All three scripts can be run independently or together via `ais_host_config.sh`.

## What It Configures

| Step | Script | Ansible Equivalent | Description |
|------|--------|--------------------|-------------|
| sysctl | `ais_host_config.sh` | `ais_host_config_sysctl` | Arbitrary sysctl key-value pairs via `/etc/sysctl.d/` drop-ins (config-file-only) |
| Block device tuning | `ais_host_config.sh` | `ais_host_config_common` (tag: `io`) | `read_ahead_kb` via udev rule (persists across reboots) |
| Filesystem creation | `ais_datafs.sh` | `ais_datafs_mkfs` | Parallel `mkfs` on specified devices |
| Filesystem mounting | `ais_datafs.sh` | `ais_datafs_mount` | Mount at `/ais/<device>`, fstab entries using UUID |

## Dependencies

| Command | Package | Used by |
|---------|---------|---------|
| `bash` | bash | All scripts |
| `udevadm` | udev (systemd) | `ais_host_config.sh` (block device tuning rules) |
| `sysctl` | procps | `ais_host_config.sh` |
| `curl` | curl | `install_deps.sh` (downloads `yq` binary) |
| `yq` | [mikefarah/yq](https://github.com/mikefarah/yq) | `ais_host_config.sh` (YAML config parsing) |
| `blkid` | util-linux | `ais_datafs.sh` |
| `mount`, `mountpoint` | util-linux | `ais_datafs.sh` |
| `mkfs.xfs` | xfsprogs | `ais_datafs.sh` (when `FSTYPE=xfs`, the default) |
| `mkfs.ext4` | e2fsprogs | `ais_datafs.sh` (when `FSTYPE=ext4`) |

Use `install_deps.sh` to install all missing dependencies, including `yq`:

```bash
sudo bash install_deps.sh
```

## Quick Start

```bash
sudo bash install_deps.sh
sudo AIS_DEVICES="nvme1n1 nvme2n1 nvme3n1 nvme4n1" bash ais_host_config.sh
```

Or run filesystem setup separately:

```bash
sudo AIS_DEVICES="nvme1n1 nvme2n1" bash ais_datafs.sh
```

## Cloud / VM Userdata

These scripts are designed to run as instance userdata (cloud-init) on first
boot. 

Reference the example AWS script for example of writing the config YAML inline and fetching the scripts directly from remote.

**AWS EKS**: See `aws-eks-userdata.sh.example` for a ready-to-paste launch
template userdata script. Copy it, edit the config section for your instance
type, and paste into the AWS console or reference from Terraform.

**Other clouds / generic**: Fetch the scripts and config to a working
directory and run them. All `.sh` scripts must be co-located in the same
directory for `ais_host_config.sh` to find `ais_datafs.sh`.

```bash
#!/bin/bash
cd /opt
for f in install_deps.sh ais_host_config.sh ais_datafs.sh my-config.yaml; do
    curl -fsSL -o "$f" "<raw-url>/$f"
done
chmod +x install_deps.sh ais_host_config.sh ais_datafs.sh
bash install_deps.sh
AIS_CONFIG=/opt/my-config.yaml bash ais_host_config.sh
```

## Configuration File

Instead of passing many environment variables, you can provide a YAML config
file. 
Copy and edit the example:

```bash
cp config.yaml.example my-cluster.yaml
vim my-cluster.yaml
sudo AIS_CONFIG=my-cluster.yaml bash ais_host_config.sh
```

Environment variables always override config file values, so you can use
a config file as a baseline and tweak individual settings at run time:

```bash
sudo AIS_CONFIG=my-cluster.yaml SKIP_MKFS=true bash ais_host_config.sh
```

See `config.yaml.example` for the full structure and comments. Every field
is optional — omitted fields use the script defaults.

## Customization

All settings are controlled via environment variables (or YAML config file)
with sensible defaults.

### Devices & Mounts

| Variable | Default | Description |
|----------|---------|-------------|
| `AIS_DEVICES` | *(required)* | Space-separated device names, e.g. `"nvme1n1 nvme2n1"` |
| `MPATH_PREFIX` | `/ais` | Mount base path — each device mounts at `<prefix>/<device>` |
| `FSTYPE` | `xfs` | Filesystem type |
| `FS_MOUNT_OPTIONS` | XFS-optimized | Mount options written to fstab |
| `SKIP_MKFS` | `false` | Set `true` to mount existing filesystems without reformatting |

### Block Device Tuning

| Variable | Default | Description |
|----------|---------|-------------|
| `BLKDEVTUNE_PATTERN` | `nvme*` | Kernel name glob for devices to tune (udev `KERNEL==` match, e.g. `nvme*`, `sd*`, `nvme[12]n1`) |
| `BLKDEV_READ_AHEAD_KB` | `N/A` | `read_ahead_kb` value |

### Sysctl Tuning

Sysctl settings are defined **only** in the YAML config file — there are no
environment variable overrides for individual sysctl knobs. Each sub-key under
`sysctl:` becomes a `/etc/sysctl.d/99-ais-<name>.conf` drop-in file, and its
entries are arbitrary sysctl key-value pairs passed through verbatim.

Omit a category to skip it; remove the `sysctl:` key entirely (or set
`SKIP_SYSCTL=true`) to skip all sysctl configuration. When no `AIS_CONFIG` is
provided, sysctl configuration is skipped automatically.

See `config.yaml.example` for recommended defaults organized into `required`,
`net`, and `vm` categories. You can rename, add, or remove categories freely.

Be sure to update these carefully to account for the available cpu, memory, and networking on your instance shapes.  

### Feature Flags

| Variable | Default | Description |
|----------|---------|-------------|
| `SKIP_SYSCTL` | `false` | Skip all sysctl configuration |
| `SKIP_BLKDEVTUNE` | `false` | Skip block device tuning (udev rule) |
| `SKIP_MKFS` | `false` | Skip filesystem creation (mount only) |

## Examples

**Using a config file:**

```bash
sudo AIS_CONFIG=my-cluster.yaml bash ais_host_config.sh
```

**NVMe cloud instance (4 data drives):**

```bash
sudo AIS_DEVICES="nvme1n1 nvme2n1 nvme3n1 nvme4n1" bash ais_host_config.sh
```

**Filesystem setup only (no host tuning):**

```bash
sudo AIS_DEVICES="nvme1n1 nvme2n1 nvme3n1" bash ais_datafs.sh
```

**HDD instance with custom mount path:**

```bash
sudo AIS_DEVICES="sdb sdc sdd sde sdf sdg sdh sdi" \
     MPATH_PREFIX="/data" \
     BLKDEVTUNE_PATTERN="sd*" \
     bash ais_host_config.sh
```

**Mount pre-existing filesystems (no reformat):**

```bash
sudo AIS_DEVICES="nvme1n1 nvme2n1" SKIP_MKFS=true bash ais_host_config.sh
```

**Sysctl-only (no disks, no block tuning):**

```bash
sudo AIS_CONFIG=my-cluster.yaml SKIP_BLKDEVTUNE=true bash ais_host_config.sh
```

**Custom sysctl values (e.g. 50 Gbps network, 512 GB memory host):**

Edit your YAML config to adjust the sysctl values directly — see
`config.yaml.example` for the full structure.

```bash
sudo AIS_CONFIG=my-50g-host.yaml AIS_DEVICES="nvme0n1 nvme1n1" bash ais_host_config.sh
```
