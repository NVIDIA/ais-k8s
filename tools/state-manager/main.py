#
# Copyright (c) 2025, NVIDIA CORPORATION. All rights reserved.
#
import argparse
import sys
from pathlib import Path
from typing import List, Optional

from ais_metadata import AISMetadata
from backup_creator import BackupCreator
from cleanup_runner import CleanupRunner
from deletion_runner import DeletionRunner
from k8s_manager import K8sManager
from pod_config import PodConfig
from restore_runner import RestoreRunner

ACTIONS = ["backup", "delete", "restore", "clean-pv", "clean-host"]


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


def clean_pv(manager: K8sManager, clean_state: bool, clean_data: bool,
             storage_class: str):
    """
    Cleans state (.ais.*) and/or data (@*) from PVs identified by storage class.
    :param manager: K8s manager for interfacing with k8s
    :param clean_state: Whether to remove .ais.* state files
    :param clean_data: Whether to remove @* data directories
    :param storage_class: Storage class to identify PVs to clean
    """
    cmds = []
    if clean_state:
        cmds.append("rm -rf /data/.ais.*")
    if clean_data:
        cmds.append("rm -rf /data/@*")
    pod_config = PodConfig(
        name=f"clean-{manager.cluster_name}-" + "{pvc_name}",
        image="busybox:latest",
        container_name="clean",
        command=["sleep", "3600"],
        exec_cmd=" && ".join(cmds),
        labels={"app.kubernetes.io/name": f"{manager.cluster_name}-clean"},
    )
    runner = DeletionRunner(manager, pod_config, storage_class=storage_class)
    manager.delete_pods(pod_config)
    runner.delete()


def clean_host(manager: K8sManager, clean_state: bool, clean_data: bool,
               mount_paths: list, hostpath_prefix: Optional[str] = None):
    """
    Cleans state (.ais.*) and/or data (@*) from host disk paths.
    :param manager: K8s manager for interfacing with k8s
    :param clean_state: Whether to remove .ais.* state files
    :param clean_data: Whether to remove @* data directories
    :param mount_paths: Host mount paths for target data
    :param hostpath_prefix: Host path prefix for proxy state
    """
    pod_config = PodConfig(
        name=f"clean-host-{manager.cluster_name}-" + "{role}-{node_name}",
        image="busybox:latest",
        container_name="clean",
        command=[],
        exec_cmd="",
        labels={"app.kubernetes.io/name": f"{manager.cluster_name}-clean"},
    )
    runner = CleanupRunner(manager, pod_config, clean_state, clean_data)
    manager.delete_pods(pod_config)
    runner.clean_host(mount_paths, hostpath_prefix=hostpath_prefix)


def create_arg_parser():
    parser = argparse.ArgumentParser(
        description="Manage AIS cluster storage: backup, restore, delete metadata, and clean state/data"
    )
    parser.add_argument("--kube-context", type=str, help="Kubernetes context to use")
    parser.add_argument("--namespace", type=str, help="Namespace of the cluster")
    parser.add_argument("--cluster", type=str, help="Cluster name")
    parser.add_argument(
        "--action",
        type=str,
        choices=ACTIONS,
        help="Action to perform",
    )
    parser.add_argument(
        "--delete-md",
        type=str,
        help=f"Comma-separated metadata types ({AISMetadata.get_options_str()}) for deletion",
    )
    parser.add_argument(
        "--restore-src", type=str, help="Source backup tgz file to restore"
    )
    parser.add_argument(
        "--state", action="store_true",
        help="(clean-pv/clean-host) Remove .ais.* state files",
    )
    parser.add_argument(
        "--data", action="store_true",
        help="(clean-pv/clean-host) Remove @* data/bucket directories",
    )
    parser.add_argument(
        "--storage-class", type=str,
        help="(clean-pv) Storage class to identify PVs to clean",
    )
    parser.add_argument(
        "--mount-paths", type=str,
        help="(clean-host) Comma-separated host mount paths (e.g. /ais/sda,/ais/sdb)",
    )
    parser.add_argument(
        "--hostpath-prefix", type=str,
        help="(clean-host) Host path prefix for proxy state (e.g. /etc/ais)",
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
        while True:
            actions_str = "/".join(ACTIONS)
            action_input = (
                input(f"Enter action ({actions_str}): ").strip().lower()
            )
            if action_input in ACTIONS:
                args.action = action_input
                break
            print(f"Error: Invalid action. Please enter one of: {actions_str}")


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


def process_clean_state_data_args(args):
    if not args.state and not args.data:
        print("\nSelect what to clean:")
        print("  1. State only (.ais.* files)")
        print("  2. Data only (@* directories)")
        print("  3. Both state and data")
        choice = input("Choice (1/2/3): ").strip()
        if choice == "1":
            args.state = True
        elif choice == "2":
            args.data = True
        elif choice == "3":
            args.state = True
            args.data = True
        else:
            print("Invalid choice.")
            sys.exit(1)


def process_clean_pv_args(args):
    process_clean_state_data_args(args)
    if not args.storage_class:
        args.storage_class = input("Enter storage class: ").strip()
    if not args.storage_class:
        print("Error: storage class cannot be empty.")
        sys.exit(1)


def process_clean_host_args(args):
    process_clean_state_data_args(args)
    if not args.mount_paths:
        args.mount_paths = input(
            "Enter comma-separated mount paths (e.g. /ais/sda,/ais/sdb): "
        ).strip()
    if not args.mount_paths:
        print("Error: mount paths cannot be empty.")
        sys.exit(1)
    if not args.hostpath_prefix:
        args.hostpath_prefix = input(
            "Enter host path prefix for state (optional, press enter to skip): "
        ).strip() or None


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
    elif args.action == "clean-pv":
        process_clean_pv_args(args)
        clean_pv(
            manager,
            clean_state=args.state,
            clean_data=args.data,
            storage_class=args.storage_class,
        )
    elif args.action == "clean-host":
        process_clean_host_args(args)
        mount_paths = [p.strip() for p in args.mount_paths.split(",") if p.strip()]
        clean_host(
            manager,
            clean_state=args.state,
            clean_data=args.data,
            mount_paths=mount_paths,
            hostpath_prefix=args.hostpath_prefix,
        )


if __name__ == "__main__":
    main()
