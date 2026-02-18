# OS Hardening Playbook for CISCAT Compliance (Oracle Linux 8)

This Ansible playbook is designed to harden the operating system to meet security compliance requirements for CISCAT scans. 
The playbook includes various roles and tasks to configure system settings, install necessary packages, and enforce security policies.

## Roles and Tasks

### 1. AIDE
- Installs and initializes AIDE (Advanced Intrusion Detection Environment).
- Creates and enables systemd services and timers for AIDE.
- Ensures cryptographic mechanisms are used to protect the integrity of audit tools.

### 2. Cron
- Sets ownership and permissions for cron-related directories and files.
- Ensures crontab is restricted to authorized users.

### 3. Crypto Policies
- Configures system-wide cryptographic policies to disable SHA1, CBC mode ciphers for SSH, and weak MAC algorithms.

### 4. Filesystem Modules
- Prevent loading and blacklist the `cramfs` module.
- Prevent loading and blacklist the `jffs2` module.
- Prevent loading and blacklist the `usb-storage` module.
- Unload these modules from the running kernel.

### 5. Journald Configuration
- Ensure journald is configured to compress large log files.
- Ensure journald is configured to write logfiles to persistent disk

### 6. Kernel
- Ensures kernel sysctl settings (`randomize_va_space` and `yama.ptrace_scope`) are persisted and applied.

### 7. Network Kernel
- Configures and enforces various network-related sysctl settings.
- Stops and removes `rpcbind` package.

### 8. Password Policies (PAM)
- Sets password lockout policies.
- Ensures password quality requirements are enforced.
- Sets password history requirements.

### 9. Profile Settings
- Configures UMASK settings to be more restrictive.
- Ensures default user shell timeout is configured.
- Configures sysctl settings for kernel parameters.

### 10. RSYSLOG
- Configures default file permissions for rsyslog and ensures the service is restarted.

### 11. SSHD
- Set ownership and permissions for SSH configuration files.
- Ensure sshd settings are configured.

### 12. Sudo
- Ensures sudo commands use a pseudo-terminal (pty).
- Ensures sudo log file exists and is properly configured.

### 13. `/tmp` 
- Ensure /tmp is a separate partition.


## How to Run
1. **Create an inventory file:**
   Create a file named `inventory.ini` and list your target machines:
   ```ini
   [all]
   server1(host-name) ansible_host=10.49.41.111
   ```

2. **Run the playbook:**
   ```bash
   ansible-playbook -i inventory.ini os_hardening.yaml
   ```
