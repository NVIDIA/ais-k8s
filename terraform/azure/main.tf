provider "azurerm" {
  features {}
}

variable "location" {
  description = "The Azure Region in which all resources in this example should be provisioned"
}

resource "azurerm_resource_group" "example" {
  name     = "ais"
  location = var.location
}

resource "azurerm_kubernetes_cluster" "example" {
  name                = "ais-k8s"
  location            = azurerm_resource_group.example.location
  resource_group_name = azurerm_resource_group.example.name
  dns_prefix          = "ais-k8s"

  default_node_pool {
    name       = "default"
    node_count = 1
    vm_size    = "Standard_DS2_v2"
  }

  identity {
    type = "SystemAssigned"
  }

  addon_profile {
    aci_connector_linux {
      enabled = false
    }

    azure_policy {
      enabled = false
    }

    http_application_routing {
      enabled = false
    }

    kube_dashboard {
      enabled = true
    }

    oms_agent {
      enabled = false
    }
  }
}
