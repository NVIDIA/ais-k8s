#
# Copyright (c) 2025, NVIDIA CORPORATION. All rights reserved.
#
import sys
from typing import List, Optional

from ais_metadata import AISMetadata

# Metadata types that only exist on proxy state PVCs
# All proxy state metadata is included in backup/restore
PROXY_ONLY_MD = {AISMetadata.rmd}

class DeletionRunner(object):
    def __init__(self, manager, pod_config,
                 metadata: Optional[List[AISMetadata]] = None,
                 storage_class: Optional[str] = None):
        self.manager = manager
        self.pod_config = pod_config
        self.storage_class = storage_class
        self.proxy_only = False
        if metadata:
            self.proxy_only = all(md in PROXY_ONLY_MD for md in metadata)
            self.pod_config.exec_cmd = self.get_deletion_cmd(metadata)

    @staticmethod
    def get_deletion_cmd(metadata: List[AISMetadata]):
        values = " /data/".join([md.value for md in metadata])
        return f"rm /data/{values}"

    def delete(self):
        self.manager.confirm_cluster_not_running()
        pvcs = self.manager.find_pvcs(
            proxy_only=self.proxy_only, storage_class=self.storage_class
        )
        if not pvcs:
            sys.exit("No PVCs found. Aborting.")
        print("Deploying pods")
        self.manager.create_pods(self.pod_config, pvcs)
        self.manager.wait_for_pods_status(self.pod_config)
        print("Running task")
        self.manager.exec_command(self.pod_config, pvcs)
        print("Deleting pods")
        self.manager.delete_pods(self.pod_config)
        print("Complete.")
