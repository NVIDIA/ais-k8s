## Chart Generation Workflow

This directory is used to supplement Helm chart generation using [Helmify](https://github.com/arttor/helmify).

Helmify allows us to use the following workflow to automatically generate a helm chart: 

1. [controller-gen](https://book.kubebuilder.io/reference/controller-gen.html)
2. [Kustomize](https://kustomize.io/)
3. Helmify 
4. Helm chart `ais-operator`

The `make build-installer-helm` target will copy generated templates and values into the persisted chart in [helm/ais-operator](./ais-operator). 

Our Kustomize overlay allows us to inject placeholders into the manifests that are passed into Helmify. 
We can use these placeholders to inject additional templates beyond what Helmify does by default.
This allows us to both automatically generate a Helm chart from generated code while also maintaining a long-lived chart with our own versioning, notes, helpers, etc.

The [apply_replacements](./apply_replacements.sh) script merges additional values from the [provided values file](./replacements/values.yaml) into the generated one.
It also uses [gomplate](https://github.com/hairyhenderson/gomplate) to inject helm templating in addition to what's done by Helmify. 
This allows us to add additional Helm templating where needed. 


## Why Templated Replacements

Hopefully this becomes unnecessary over time with development of Helmify and/or the alpha [kubebuilder helm plugin](https://book.kubebuilder.io/plugins/available/helm-v2-alpha). 
As of writing, neither option provides the level of chart customization we need.

Kustomize itself cannot directly inject Helm templating into the output manifests.
Tools like yq and ytt also struggle with any non-standard yaml format introduced by Helm templates. 