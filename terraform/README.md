See: https://learn.hashicorp.com/tutorials/terraform/gke?in=terraform/kubernetes

# Learn Terraform - Provision a GKE Cluster

This repo is a companion repo to the [Provision a GKE Cluster learn guide](https://learn.hashicorp.com/terraform/kubernetes/provision-gke-cluster), containing
Terraform configuration files to provision an GKE cluster on
GCP.

This sample repo also creates a VPC and subnet for the GKE cluster. This is not
required but highly recommended to keep your GKE cluster isolated.

## Install and configure GCloud

First, install the [Google Cloud CLI](https://cloud.google.com/sdk/docs/quickstarts) 
and initialize it.

```shell
$ gcloud init
```

Once you've initialized gcloud (signed in, selected project), add your account 
to the Application Default Credentials (ADC). This will allow Terraform to access
these credentials to provision resources on GCloud.

```shell
$ gcloud auth application-default login
```

## Initialize Terraform workspace and provision GKE Cluster

Replace `terraform.tfvars` values with your `project_id` and `region`. Your 
`project_id` must match the project you've initialized gcloud with. To change your 
`gcloud` settings, run `gcloud init`. The region has been defaulted to `us-central1`;
you can find a full list of gcloud regions [here](https://cloud.google.com/compute/docs/regions-zones).

After you've done this, initalize your Terraform workspace, which will download 
the provider and initialize it with the values provided in the `terraform.tfvars` file.

```shell
$ terraform init

Initializing the backend...

Initializing provider plugins...
- Checking for available provider plugins...
- Downloading plugin for provider "google" (hashicorp/google) 3.13.0...
Terraform has been successfully initialized!
```


Then, provision your AKS cluster by running `terraform apply`. This will 
take approximately 10 minutes.

```shell
$ terraform apply

# Output truncated...

Plan: 4 to add, 0 to change, 0 to destroy.

Do you want to perform these actions?
  Terraform will perform the actions described above.
  Only 'yes' will be accepted to approve.

# Output truncated...

Apply complete! Resources: 4 added, 0 changed, 0 destroyed.

Outputs:

kubernetes_cluster_name = dos-terraform-edu-gke
region = us-central1
```

## Configure kubectl

To configure kubetcl, by running the following command. 

```shell
$ gcloud container clusters get-credentials dos-terraform-edu-gke --region us-central1
```

The
[Kubernetes Cluster Name](https://github.com/hashicorp/learn-terraform-provision-gke-cluster/blob/master/gke.tf#L63)
and [Region](https://github.com/hashicorp/learn-terraform-provision-gke-cluster/blob/master/vpc.tf#L29)
 correspond to the resources spun up by Terraform.

## Deploy and access Kubernetes Dashboard

To deploy the Kubernetes dashboard, run the following command. This will schedule 
the resources necessary for the dashboard.

```shell
kubectl apply -f https://raw.githubusercontent.com/kubernetes/dashboard/v2.0.0-beta8/aio/deploy/recommended.yaml
namespace/kubernetes-dashboard created
serviceaccount/kubernetes-dashboard created
service/kubernetes-dashboard created
secret/kubernetes-dashboard-certs created
secret/kubernetes-dashboard-csrf created
secret/kubernetes-dashboard-key-holder created
configmap/kubernetes-dashboard-settings created
role.rbac.authorization.k8s.io/kubernetes-dashboard created
clusterrole.rbac.authorization.k8s.io/kubernetes-dashboard created
rolebinding.rbac.authorization.k8s.io/kubernetes-dashboard created
clusterrolebinding.rbac.authorization.k8s.io/kubernetes-dashboard created
deployment.apps/kubernetes-dashboard created
service/dashboard-metrics-scraper created
deployment.apps/dashboard-metrics-scraper created
```

Finally, to access the Kubernetes dashboard, run the following command:

```plaintext
$ kubectl proxy
Starting to serve on 127.0.0.1:8001
```

 You should be
able to access the Kubernetes dashboard at [http://127.0.0.1:8001/api/v1/namespaces/kubernetes-dashboard/services/https:kubernetes-dashboard:/proxy/](http://127.0.0.1:8001/api/v1/namespaces/kubernetes-dashboard/services/https:kubernetes-dashboard:/proxy/).

## Authenticate to Kubernetes Dashboard

To view the Kubernetes dashboard, you need to provide an authorization token. 
Authenticating using `kubeconfig` is **not** an option. You can read more about
it in the [Kubernetes documentation](https://kubernetes.io/docs/tasks/access-application-cluster/web-ui-dashboard/#accessing-the-dashboard-ui).

Generate the token in another terminal (do not close the `kubectl proxy` process).

```plaintext
kubectl -n kube-system describe secret $(kubectl -n kube-system get secret | grep service-controller-token | awk '{print $1}')

Name:         service-controller-token-m8m7j
Namespace:    kube-system
Labels:       <none>
Annotations:  kubernetes.io/service-account.name: service-controller
              kubernetes.io/service-account.uid: bc99ddad-6be7-11ea-a3c7-42010a800017
              
Type:  kubernetes.io/service-account-token

Data
====
namespace:  11 bytes
token:      eyJhbGciOiJSUzI1NiIsImtpZCI6IiJ9...
ca.crt:     1119 bytes
```

Select "Token" then copy and paste the entire token you receive into the 
[dashboard authentication screen](http://127.0.0.1:8001/api/v1/namespaces/kubernetes-dashboard/services/https:kubernetes-dashboard:/proxy/) 
to sign in. You are now signed in to the dashboard for your Kubernetes cluster.

