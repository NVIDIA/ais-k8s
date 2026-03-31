#
# Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
#
import sys


class CleanupRunner:
    def __init__(self, manager, pod_config, clean_state, clean_data):
        self.manager = manager
        self.pod_config = pod_config
        self.clean_state = clean_state
        self.clean_data = clean_data

    def _build_hostpath_command(self, role, mount_paths, hostpath_prefix):
        cmds = []
        if role == "proxy" and self.clean_state and hostpath_prefix:
            cmds.append(f"rm -rf {hostpath_prefix}/.ais.*")
        if role == "target":
            if self.clean_state and hostpath_prefix:
                cmds.append(f"rm -rf {hostpath_prefix}/.ais.*")
            for path in mount_paths:
                if self.clean_state:
                    cmds.append(f"rm -rf {path}/.ais.*")
                if self.clean_data:
                    cmds.append(f"rm -rf {path}/@*")
        return "\n".join(cmds) if cmds else None

    def _build_hostpath_volumes(self, role, mount_paths, hostpath_prefix):
        volumes = []
        if hostpath_prefix:
            volumes.append(("state", hostpath_prefix))
        if role == "target":
            for i, path in enumerate(mount_paths):
                volumes.append((f"mpath-{i}", path))
        return volumes

    def clean_host(self, mount_paths, hostpath_prefix=None):
        self.manager.confirm_cluster_not_running()
        proxy_nodes = self.manager.find_nodes_by_label(
            f"nvidia.com/ais-proxy={self.manager.cluster_name}"
        )
        target_nodes = self.manager.find_nodes_by_label(
            f"nvidia.com/ais-target={self.manager.cluster_name}"
        )
        if not proxy_nodes and not target_nodes:
            sys.exit("No proxy or target nodes found. Are nodes labeled?")

        print(f"Proxy nodes: {proxy_nodes}")
        print(f"Target nodes: {target_nodes}")

        for role, nodes in [("proxy", proxy_nodes), ("target", target_nodes)]:
            cmd = self._build_hostpath_command(role, mount_paths, hostpath_prefix)
            if not cmd or not nodes:
                continue
            volumes = self._build_hostpath_volumes(role, mount_paths, hostpath_prefix)
            self.manager.create_hostpath_pods(
                self.pod_config, nodes, volumes, cmd, role=role
            )

        print("Waiting for clean pods to complete...")
        self.manager.wait_for_pods_status(
            self.pod_config, desired_status="Succeeded", timeout=120
        )
        print("Deleting clean pods")
        self.manager.delete_pods(self.pod_config)
        print("Host clean complete.")
