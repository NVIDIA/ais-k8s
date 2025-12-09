#
# Copyright (c) 2025, NVIDIA CORPORATION. All rights reserved.
#
import sys
from typing import List

from ais_metadata import AISMetadata

# Metadata types that only exist on proxy state PVCs
# All proxy state metadata is included in backup/restore
PROXY_ONLY_MD = {AISMetadata.rmd}

class DeletionRunner(object):
    def __init__(self, manager, pod_config, metadata: List[AISMetadata]):
        self.manager = manager
        self.pod_config = pod_config
        self.proxy_only = all(md in PROXY_ONLY_MD for md in metadata)
        self.pod_config.exec_cmd = self.get_deletion_cmd(metadata)

    @staticmethod
    def get_deletion_cmd(metadata: List[AISMetadata]):
        values = " /data/".join([md.value for md in metadata])
        return f"rm /data/{values}"

    def delete(self):
        print("Checking for running cluster")
        if self.manager.is_cluster_running():
            proceed = input(
                "WARNING -- Cluster is still running. Are you sure you want to proceed with deletion? (y/n): "
            )
            if proceed.lower() != "y":
                print("Aborting deletion as requested.")
                sys.exit(1)
            else:
                print("Proceeding with deletion.")
        pvcs = self.manager.find_pvcs(proxy_only=self.proxy_only)
        print("Deploying deletion pods")
        self.manager.create_pods(self.pod_config, pvcs)
        self.manager.wait_for_pods_status(self.pod_config)
        print(f"Running deletion task on metadata")
        self.manager.exec_command(self.pod_config, pvcs)
        print("Deleting deletion pods")
        self.manager.delete_pods(self.pod_config)
        print("Metadata deletion complete.")
