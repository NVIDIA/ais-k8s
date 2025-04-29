#
# Copyright (c) 2025, NVIDIA CORPORATION. All rights reserved.
#
import os
import sys
import tarfile
from pathlib import Path
from kubernetes import client
from pod_config import PodConfig


class RestoreRunner:
    def __init__(self, manager, src_path: Path, pod_config: PodConfig):
        self.manager = manager
        self.src_path = src_path
        self.pod_config = pod_config
        self.pvc_backups = self.init_pvc_backups_dir()

    @staticmethod
    def init_pvc_backups_dir():
        pvc_backups = Path("restore").joinpath("pvc_backups")
        pvc_backups.mkdir(parents=True, exist_ok=True)
        return pvc_backups

    @property
    def cluster_name(self):
        return self.manager.cluster_name

    @property
    def cluster_ns(self):
        return self.manager.cluster_ns

    def extract_restore_file(self):
        print(f"Extracting {self.src_path}")
        with tarfile.open(self.src_path, 'r:gz') as tar:
            tar.extractall(path=self.pvc_backups)

    #TODO: This should use backed-up PVC settings to copy the original
    def create_pvcs(self, pvc_names):
        for pvc in pvc_names:
            component = "proxy"
            if "target" in pvc:
                component = "target"
            print("Creating PVC", pvc)
            pvc_manifest = client.V1PersistentVolumeClaim(
                metadata=client.V1ObjectMeta(name=pvc, labels={
                    "app.kubernetes.io/name": self.manager.cluster_name,
                    "app.kubernetes.io/component": component
                }),
                spec=client.V1PersistentVolumeClaimSpec(
                    access_modes=["ReadWriteOnce"],
                    storage_class_name="local-path",
                    resources=client.V1ResourceRequirements(
                        requests={"storage": "1Gi"}
                    )
                )
            )
            self.manager.create_pvc(pvc_manifest)


    def validate_pvcs(self):
        pvcs = self.manager.find_pvcs()
        # From the restore file, figure out desired PVC names
        pvc_backup_files = self.pvc_backups.glob('*.tar.gz')
        desired_pvcs = []
        for backup in pvc_backup_files:
            desired_pvcs.append(backup.name.rstrip(".tar.gz"))
        # If we have all, we're done
        if set(pvcs) == set(desired_pvcs):
            print(f"Found PVCs matching restore files: {pvcs}")
            return desired_pvcs
        # If we have 0, create them
        if len(pvcs) == 0:
            print(f"No pvcs found, creating {desired_pvcs}")
            self.create_pvcs(desired_pvcs)
            return desired_pvcs
        # If it doesn't match, probably an invalid restore file for this cluster, abort
        sys.exit(f"Found non-zero existing PVCs not matching restore file. Desired: {desired_pvcs}. Actual: {pvcs}")

    def copy_to_pod(self, pod_name, remote_path, local_path):
        # Uses 'kubectl cp' as the Python client does not provide file copy
        cmd = f"kubectl cp {local_path} {self.manager.cluster_ns}/{pod_name}:{remote_path}"
        print(f"Copying backup to pod: {cmd}")
        if os.system(cmd) != 0:
            sys.exit(f"Failed to copy backup to {pod_name}")

    def restore_data(self, pvcs):
        for pvc in pvcs:
            pod_name = self.pod_config.name.format(pvc_name=pvc)
            remote_path = f"/{pvc}.tar.gz"
            local_path = self.pvc_backups.joinpath(f"{pvc}.tar.gz")
            self.copy_to_pod(pod_name, remote_path, local_path)
        self.manager.exec_command(self.pod_config, pvcs)

    def restore(self):
        print("Checking for running cluster")
        if self.manager.is_cluster_running():
            sys.exit("Aborting restore -- cluster is running")
        print("Extract local restore file")
        self.extract_restore_file()
        print("Validating PVCs")
        pvcs = self.validate_pvcs()
        # At this point, we can create pvcs based on the backup names
        print("Deploying restore pods")
        self.manager.create_pods(self.pod_config, pvcs)
        self.manager.wait_for_pods_status(self.pod_config)
        print("Copying data tars and extracting in restore pods")
        self.restore_data(pvcs)
        print("Deleting restore pods")
        self.manager.delete_pods(self.pod_config)
        print("Restore complete.")