#
# Copyright (c) 2025, NVIDIA CORPORATION. All rights reserved.
#
import sys
import time
from datetime import datetime, timedelta

from kubernetes import client, config
from kubernetes.stream import stream as k8s_stream

from pod_config import PodConfig


class K8sManager:
    def __init__(self, kube_context, cluster_ns, cluster_name):
        self.k8s_client = self.init_k8s_client(kube_context)
        self.cluster_ns = cluster_ns
        self.cluster_name = cluster_name
        self.ais_label_selector = (
            f"app.kubernetes.io/component in (proxy,target),app.kubernetes.io/name={self.cluster_name}"
        )

    @staticmethod
    def init_k8s_client(kube_context) -> client.CoreV1Api:
        config.load_kube_config(context=kube_context)
        return client.CoreV1Api()

    def define_pod_manifest(self, pod_config, pvc_name):
        return {
            "apiVersion": "v1",
            "kind": "Pod",
            "metadata": {"name": pod_config.name.format(pvc_name=pvc_name), "namespace": self.cluster_ns, "labels": pod_config.labels},
            "spec": {
                "containers": [
                    {
                        "name": pod_config.container_name,
                        "image": pod_config.image,
                        "command": pod_config.command,
                        "volumeMounts": [{"mountPath": "/data", "name": "pvc-volume"}],
                    }
                ],
                "volumes": [{"name": "pvc-volume", "persistentVolumeClaim": {"claimName": pvc_name}}],
                "restartPolicy": "Never"
            },
        }

    def create_pods(self, pod_config, pvc_names):
        for pvc in pvc_names:
            manifest = self.define_pod_manifest(pod_config, pvc)
            self.k8s_client.create_namespaced_pod(namespace=self.cluster_ns, body=manifest)

    def create_pvc(self, manifest):
        self.k8s_client.create_namespaced_persistent_volume_claim(namespace=self.cluster_ns, body=manifest)

    def wait_for_pods_status(self, pod_config, desired_status="Running", timeout=30, poll_interval=5):
        """
        Waits until all pods matching a label reach the specified status or timeout occurs.
        :param pod_config: Pod configuration defining which pod to wait on
        :param desired_status: The pod status to wait for (e.g., 'Running', 'Succeeded').
        :param timeout: Maximum time to wait in seconds.
        :param poll_interval: Time between status checks in seconds.
        """
        start_time = datetime.now()
        while datetime.now() - start_time < timedelta(seconds=timeout):
            pods = self.list_pods_matching_label(pod_config.label_selector)
            all_ready = True
            for pod in pods.items:
                if pod.status.phase != desired_status:
                    all_ready = False
                    break
            if all_ready:
                print(f"All pods reached status: {desired_status}")
                return
            time.sleep(poll_interval)
        sys.exit(f"Timed out waiting for pods to reach status: {desired_status}")

    def exec_command(self, pod_config: PodConfig, pvcs):
        for pvc in pvcs:
            pod_name = pod_config.name.format(pvc_name=pvc)
            exec_command = [
                "/bin/sh",
                "-c",
                pod_config.exec_cmd.format(name=pvc)
            ]
            print(f"Executing command {exec_command} in pod {pod_name}...")
            resp = k8s_stream(
                self.k8s_client.connect_get_namespaced_pod_exec,
                pod_name,
                self.cluster_ns,
                command=exec_command,
                container=pod_config.container_name,
                stderr=True,
                stdin=False,
                stdout=True,
                tty=False,
            )
            print(resp)

    def find_pvcs(self):
        pvc_list = self.k8s_client.list_namespaced_persistent_volume_claim(
            namespace=self.cluster_ns, label_selector=self.ais_label_selector
        )
        # Filter out target storage PVCs
        pvc_names = [
            pvc.metadata.name
            for pvc in pvc_list.items
            if pvc.spec.storage_class_name != "ais-local-storage"
        ]

        if not pvc_names:
            print(f"No PVCs found matching label selector {self.ais_label_selector}")
        print ("Found pvcs: ", pvc_names)
        return pvc_names

    def list_pods_matching_label(self, label_selector):
        return self.k8s_client.list_namespaced_pod(self.cluster_ns, label_selector=label_selector)

    def list_ais_pods(self):
        return self.list_pods_matching_label(self.ais_label_selector)

    def wait_for_pods_deleted(self, pod_config, timeout=60, poll_interval=5):
        start_time = datetime.now()
        while datetime.now() - start_time < timedelta(seconds=timeout):
            pods = self.list_pods_matching_label(pod_config.label_selector)
            if len(pods.items) == 0:
                return
            time.sleep(poll_interval)
        sys.exit(f"Timed out waiting for pods to be deleted")

    def delete_pods(self, pod_config):
        pods = self.list_pods_matching_label(pod_config.label_selector)
        for pod in pods.items:
            pod_name = pod.metadata.name
            print(f"Deleting pod: {pod_name}")
            # Delete the pod
            self.k8s_client.delete_namespaced_pod(name=pod_name, namespace=self.cluster_ns)
        self.wait_for_pods_deleted(pod_config)

    def is_cluster_running(self):
        pods = self.list_pods_matching_label(self.ais_label_selector)
        return len(pods.items) > 0