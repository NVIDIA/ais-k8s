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
    # fstype = "xfs" # TODO: make it work
  }

  volume_binding_mode = "WaitForFirstConsumer"
  allow_volume_expansion = true


  #TODO: revisit these options
  #mount_options = ["file_mode=0700", "dir_mode=0777", "mfsymlinks", "uid=1000", "gid=1000", "nobrl", "cache=none"]
}

# TODO: not needed for now as we use volumeClaimTemplates directly in StatefulSet spec.
# which never really happens as it is dynamic PVC
//resource "kubernetes_persistent_volume_claim" "ais-storage-claim-30" {
//  metadata {
//    name = "ais-claim"
//  }
//
//  spec {
//    access_modes = ["ReadWriteOnce"]
//    storage_class_name = "ais"
//
//    resources {
//      requests = {
//        storage = "30Gi"
//      }
//    }
//  }
//
//  depends_on = [
//    kubernetes_storage_class.ais-storage-class
//  ]
//}


