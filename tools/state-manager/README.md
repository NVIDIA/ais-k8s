# AIS State Manager

** USE WITH CAUTION. This is an ADMIN level tool that can damage your cluster if used incorrectly ** 

This tool provides an easy way to back up, delete, and restore internal config data and metadata in an AIS cluster for recovery or rollback purposes. 

It is ONLY designed to restore to a like cluster -- with no differences from the original. 

It is currently limited to our recommended PVC configurations -- dynamically provisioned, local PVCs. 

---

## Prerequisites

- `pip install requirements.txt`
- Kubectl installed and available in path. 
- An available kube context to communicate with the desire cluster. 

## Usage

See `python main.py --help` for usage instructions or start without args and answer the prompts.

### Backup

Creates a backup tar.gz of all state files in the cluster.
The backup tar.gz contains individual entries for every state PVC in the cluster.

### Delete

This allows you to delete any specific AIS metadata cluster-wide or any individual metadata file. 

Deletion is only supported when a cluster is **not** online, because these PVCs are configured with `ReadWriteOnce` policy.

### Restore

For `restore` you MUST use a compatible tgz, as created by the backup tool. 
Place this backup file in a restore/ directory before selecting a restore option. 

Restoration is only supported when a cluster is **not** online, because these PVCs are configured with `ReadWriteOnce` policy.

---

## How it works

Each of these scripts works by checking for existing PVCs that are used for mounting AIS state. 
They create pods that mount those PVCs and then execute a specific command to access or modify the state data, then remove the pods when finished. 