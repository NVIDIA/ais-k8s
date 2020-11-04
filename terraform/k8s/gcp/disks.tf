# NOTE: see that resource if prefixed with kubernetes not provider (ie. google)
# It basically means that it requires kubernetes to be already running.

resource "kubernetes_storage_class" "ais-storage-class" {
  metadata {
    name = "ais"

    annotations = {
      "storageclass.kubernetes.io/is-default-class" = true
    }
  }

  storage_provisioner = "kubernetes.io/gce-pd"
  reclaim_policy      = "Delete" # TODO: use Retain in the future?

  parameters = {
    type = "pd-standard"
    fstype = "xfs"
  }

  volume_binding_mode = "WaitForFirstConsumer"
  allow_volume_expansion = true

  mount_options = ["noatime","nodiratime","logbufs=8","logbsize=256k","largeio","inode64","swalloc","allocsize=8192k"]
}
