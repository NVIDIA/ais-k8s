#
# Copyright (c) 2025, NVIDIA CORPORATION. All rights reserved.
#
import argparse
from pathlib import Path
from typing import List

from ais_metadata import AISMetadata
from backup_creator import BackupCreator
from deletion_runner import DeletionRunner
from k8s_manager import K8sManager
from pod_config import PodConfig
from restore_runner import RestoreRunner


def backup(manager: K8sManager):
    """
    Creates a backup tgz of the AIS instance specified by the manager.
    The tgz contains individual tgz files for the contents of every state PVC in the cluster
    :param manager: K8s manager for interfacing with k8s
    """
    pod_config = PodConfig(
        # pvc_name will be substituted per-pod
        name=f"backup-{manager.cluster_name}-" + "{pvc_name}",
        image="busybox:latest",
        container_name="backup-container",
        command=["sleep", "3600"],
        exec_cmd="tar -czvf /tmp/{name}-backup.tar.gz -C /data .",
        labels={"app.kubernetes.io/name": f"{manager.cluster_name}-backup"},
    )
    backup_creator = BackupCreator(manager, "backups", pod_config)
    manager.delete_pods(pod_config)
    return backup_creator.backup()


def delete(manager: K8sManager, metadata: List[AISMetadata]):
    """
    Deletes specified AISMetadata objects from existing PVCs when a cluster does not currently exist.
    :param manager: K8s manager for interfacing with k8s
    :param metadata: Metadata files to delete, see AISMetadata
    """
    pod_config = PodConfig(
        # pvc_name will be substituted per-pod
        name="delete-{pvc_name}",
        image="busybox:latest",
        container_name="restore-container",
        command=["sleep", "3600"],
        exec_cmd="",
        labels={"app.kubernetes.io/name": f"{manager.cluster_name}-restore"},
    )
    deletion_runner = DeletionRunner(manager, pod_config, metadata=metadata)
    manager.delete_pods(pod_config)
    deletion_runner.delete()


def restore(manager: K8sManager, src: Path):
    """
    Restores a tgz backup file placed in the restore/ directory to the PVCs of a cluster that is not currently running.
    If the PVCs do not exist, they will be created with AIS defaults.

    This process makes a few assumptions, based on a current standard AIS deployment:
    # 1. State storage uses a dynamically created PVC, which creates the PV automatically
    # 2. PVC is configured as ReadWriteOnce -- only one pod can write to the volume at a time
    # We cannot modify PVCS that are currently mounted by a running cluster, so there are two scenarios
    # 1. PVCs exist, and we must verify there is no running cluster
    # 2. PVCs do not exist, and we must create them the same way a deployment would -- new deployment will be able to mount them

    :param manager: K8s manager for interfacing with k8s
    :param src: Path to the tgz backup file
    """
    pod_config = PodConfig(
        # pvc_name will be substituted per-pod
        name="restore-{pvc_name}",
        image="busybox:latest",
        container_name="restore-container",
        command=["sleep", "3600"],
        exec_cmd="tar -xzvf /{name}.tar.gz -C /data",
        labels={"app.kubernetes.io/name": f"{manager.cluster_name}-restore"},
    )
    restore_manager = RestoreRunner(manager, src, pod_config)
    manager.delete_pods(pod_config)
    return restore_manager.restore()


def create_arg_parser():
    parser = argparse.ArgumentParser(
        description="Manage AIS cluster metadata backups, deletions, and restores"
    )
    parser.add_argument("--kube-context", type=str, help="Kubernetes context to use")
    parser.add_argument("--namespace", type=str, help="Namespace of the cluster")
    parser.add_argument("--cluster", type=str, help="Cluster name")
    parser.add_argument(
        "--action",
        type=str,
        choices=["backup", "delete", "restore"],
        help="Action to perform (backup, delete, restore)",
    )
    parser.add_argument(
        "--delete-md",
        type=str,
        help=f"Comma-separated metadata types ({AISMetadata.get_options_str()}) for deletion",
    )
    parser.add_argument(
        "--restore-src", type=str, help="Source backup tgz file to restore"
    )
    return parser


def prompt_missing(args):
    if not args.kube_context:
        args.kube_context = input("Enter Kubernetes context: ").strip()

    if not args.namespace:
        args.namespace = input("Enter cluster namespace: ").strip()

    if not args.cluster:
        args.cluster = input("Enter cluster name: ").strip()

    if not args.action:
        while True:  # Keep asking until valid action
            action_input = (
                input("Enter action (backup/delete/restore): ").strip().lower()
            )
            if action_input in ["backup", "delete", "restore"]:
                args.action = action_input
                break
            print(
                "Error: Invalid action. Please enter 'backup', 'delete', or 'restore'."
            )


def process_delete_args(args):
    if args.delete_md:
        selected = args.delete_md.split(",")
    else:
        print("\nSelect metadata to delete (comma-separated):")
        print(f"Available options: {AISMetadata.get_options_str()}")
        selected = input("Your choice: ").strip().lower().split(",")

    selected = [s.strip() for s in selected]

    if "all" in selected:
        return [AISMetadata.all]
    else:
        for s in selected:
            if s not in AISMetadata.get_options():
                raise ValueError(f"Invalid metadata option: {s}")
        return [AISMetadata[s] for s in selected if s != "all"]


def process_restore_args(args) -> Path:
    if args.restore_src:
        return Path(args.restore_src.strip())
    while True:  # Keep asking until valid action
        print("\nSelect path to a backup tgz file to restore:")
        loc = Path(input("Path: ").strip())
        if loc.exists():
            return loc


def main():
    parser = create_arg_parser()
    args = parser.parse_args()
    prompt_missing(args)

    manager = K8sManager(args.kube_context, args.namespace, args.cluster)

    if args.action == "backup":
        backup(manager)
    elif args.action == "delete":
        md = process_delete_args(args)
        delete(manager, md)
    elif args.action == "restore":
        src = process_restore_args(args)
        restore(manager, src)


if __name__ == "__main__":
    main()
