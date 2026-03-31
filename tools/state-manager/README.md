# AIS State Manager

** USE WITH CAUTION. This is an ADMIN level tool that can damage your cluster if used incorrectly ** 

This tool provides an easy way to back up, delete, and restore internal config data and metadata in an AIS cluster for recovery or rollback purposes. 

It is ONLY designed to restore to a like cluster -- with no differences from the original. 

It is currently limited to our recommended PVC configurations -- dynamically provisioned, local PVCs. 

---

## Prerequisites

- `pip install -r requirements.txt`
- Kubectl installed and available in path. 
- An available kube context to communicate with the desire cluster. 

## Usage

See `python main.py --help` for usage instructions or start without args and answer the prompts.

### Backup

Creates a backup tar.gz of all state files in the cluster.
The backup tar.gz contains individual entries for every state PVC in the cluster.

### Delete

This allows you to delete any specific AIS metadata cluster-wide or any individual metadata file. 

### Restore

For `restore` you MUST use a compatible tgz, as created by the backup tool. 
Place this backup file in a restore/ directory before selecting a restore option. 

### Clean PV

Clean state (.ais.*) and/or data (@*) from PVs identified by storage class.

### Clean Host

Clean state (.ais.*) and/or data (@*) from host disk paths using node discovery and hostPath mounts.

---

## How it works

Backup, delete, and restore work by checking for existing PVCs that are used for mounting AIS state. 
They create pods that mount those PVCs and then execute a specific command to access or modify the state data, then remove the pods when finished.

Clean-pv follows the same pattern but targets data PVs by storage class.
Clean-host discovers nodes by AIS labels, creates pods with hostPath mounts pinned to each node, and runs cleanup commands directly.