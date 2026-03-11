# Sysctl Tuner

The sysctl-tuner script prompts for system specs and provides some formatted, opinionated config to use when running the host config script. 
The output can be copied directly into the `sysctl` section of the yaml generated for a specific host type. 

Use these values with caution -- they apply some sensible defaults for cloud storage servers but should not be taken as requirements.
Review your own system defaults alongside the script output and validate the provided config changes. 