#
# Copyright (c) 2025, NVIDIA CORPORATION. All rights reserved.
#
import datetime
import os
import sys
import tarfile
from datetime import datetime
from pathlib import Path

from pod_config import PodConfig


class BackupCreator:
    def __init__(self, manager, backup_dir, pod_config: PodConfig):
        self.manager = manager
        self.backup_dir = backup_dir
        self.pod_config = pod_config

    @property
    def cluster_name(self):
        return self.manager.cluster_name

    @property
    def cluster_ns(self):
        return self.manager.cluster_ns

    @property
    def k8s_client(self):
        return self.manager.k8s_client

    def backup_data(self, pvcs):
        self.manager.exec_command(self.pod_config, pvcs)

    def copy_from_pod(self, pod_name, remote_path, local_path):
        # Uses 'kubectl cp' as the Python client does not provide file copy
        cmd = f"kubectl cp {self.cluster_ns}/{pod_name}:{remote_path} {local_path}"
        print(f"Copying backup from pod: {cmd}")
        if os.system(cmd) != 0:
            sys.exit(f"Failed to copy backup from {pod_name}")


    def fetch_backups(self, pvcs):
        timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")
        backup_name = f"{self.cluster_ns}-{self.cluster_name}-{timestamp}"
        backup_dir = Path(self.backup_dir).joinpath(backup_name)
        backup_dir.mkdir(parents=True, exist_ok=True)
        backup_file = Path(self.backup_dir).joinpath(f"{backup_name}.tar.gz")

        for pvc in pvcs:
            pod_name = self.pod_config.name.format(pvc_name=pvc)
            remote_path = f"/tmp/{pvc}-backup.tar.gz"
            local_path = backup_dir.joinpath(f"{pvc}.tar.gz")
            self.copy_from_pod(pod_name, remote_path, local_path)
        with tarfile.open(backup_file, "w:gz") as tar:
            for item in backup_dir.iterdir():
                tar.add(item, arcname=item.name)
        return backup_file

    def backup(self):
        print("Searching for pvcs")
        pvcs = self.manager.find_pvcs()
        if len(pvcs) == 0:
            sys.exit(f"No valid state pvcs found in namespace {self.cluster_ns}")
        print("Deploying backup pods")
        self.manager.create_pods(self.pod_config, pvcs)
        self.manager.wait_for_pods_status(self.pod_config)
        print("Creating data tars")
        self.backup_data(pvcs)
        print("Fetching final output")
        backup_file = self.fetch_backups(pvcs)
        print("Deleting backup pods")
        self.manager.delete_pods(self.pod_config)
        print(f"Backup complete. Output written to: {backup_file}")
        return backup_file