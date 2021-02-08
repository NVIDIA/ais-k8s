provider "kubernetes" {
  config_path = "~/.kube/config"
}

resource "kubernetes_storage_class" "ais-storage-class" {
  metadata {
    name = "ais"

    annotations = {
      "storageclass.kubernetes.io/is-default-class" = true
    }
  }

  storage_provisioner = "kubernetes.io/gce-pd"
  reclaim_policy      = "Retain"

  parameters = {
    type = "pd-standard"
    fstype = "xfs"
  }

  volume_binding_mode = "WaitForFirstConsumer"
  allow_volume_expansion = true

  mount_options = ["noatime","nodiratime","logbufs=8","logbsize=256k","largeio","inode64","swalloc","allocsize=8192k"]
}
