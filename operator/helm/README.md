This directory is used for Helm chart generation using [helmify](https://github.com/arttor/helmify).
Helmify allows us to use the following pattern to automatically generate a helm chart: `kubebuilder` -> `kustomize` -> `helmify` -> `helm charts`. 
 
We rely on kustomize, helmify, and postprocessing scripts like `scripts/patch_helm_template.sh` to generate the templates. 
Here we provide `extra-values.yaml` to match variables injected by the above process but not detected by helmify as user-customizable values in the default `values.yaml`. 