#
# Copyright (c) 2025, NVIDIA CORPORATION. All rights reserved.
#
import sys


class DeletionRunner(object):
    def __init__(self, manager, pod_config, storage_class: str):
        self.manager = manager
        self.pod_config = pod_config
        self.storage_class = storage_class

    def delete(self):
        self.manager.confirm_cluster_not_running()
        pvcs = self.manager.find_pvcs(storage_class=self.storage_class)
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
