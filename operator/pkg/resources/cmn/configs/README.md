## AIS Config Versioning

This directory provides default config values and aisnode version-specific configs to allow the operator to support deployment of older aisnode versions, starting with the transition from v3.23 to v3.24.

`aisnode` is expected to ALWAYS support the config of at least the previous version. 

However, since the operator defines an initial default config for a cluster, we cannot use all the latest values and still support older aisnode versions. 

`config.DefaultAISConf` is responsible for loading the appropriate version for the defined `aisnode`. 

### Non-official images

We ONLY parse the version tag expecting the official aisnode image versions (e.g. aistorage/aisnode:v3.23, aistorage/aisnode:v3.24).

Other image tag versions must currently support all the latest aisnode configs. 

### Defining a new version

To define a new version: 
1. Create a new versioned file and type using the `BaseClusterConfig` struct.
2. This new versioned file should use the fields from the latest `aiscmn.ClusterConfig` from the project's aistore dependency. 
2. For any incompatible changes, update the previous version's config with a new type preserving the old config structure.
   3. See the `v323FSHCConf` type example in `v323.go`.
   4. Define the specific incompatible field outside `BaseClusterConfig`, e.g. `FSHC: v323FSHC`.
   5. Update as many aisnode versions back as you want to support a direct upgrade from (at least one).
6. Update the logic in `config.DefaultAISConf` to select the appropriate default config.

### Future plans

Due to the limitations of parsing the image tag and the overhead of config management, we do not intend to maintain this long-term. 

Future work is planned to move default configs outside the operator. 