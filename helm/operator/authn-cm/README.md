# AIS Operator authn-cm Helm Chart

## DEPRECATED -- Use spec options to configure auth

See the [auth docs](../../../docs/authn.md) for more info. 

---

This small helper chart sets up a ConfigMap to tell the Operator how to access AuthN clusters for each AIS cluster it may support. 

The ConfigMap name is hardcoded to `ais-operator-authn` which is the default value included in operator deployments >= v2.6.0. 

The resulting ConfigMap is loaded and referenced in [the Operator's authN client](../../../operator/pkg/services/authn_api.go).

It expects a top-level "config" value, with entries for each cluster. 
The format for each entry is `<clusterNamespace>`-`<clusterName>`. 
See the `authn` environment's [values file](../config/authn-cm/authn.yaml) for reference.

The supported values for each cluster are:

- `tls`
  - Boolean value for whether the AuthN server uses TLS 
  - `false` if not provided
- `host`
  - String value for AuthN server Hostname
  - Operator defaults to `ais-authn.ais` if not provided
- `port`
  - String value for AuthN server port
  - Operator defaults to `52001` if not provided
- `secretNamespace`
  - String value for K8s namespace of the AuthN admin credentials secret
  - Must be provided
- `secretName`
  - String value for name of the AuthN admin credentials secret
  - Secret is expected to contain entries for `SU-NAME` and `SU-PASS`
  - Must be provided