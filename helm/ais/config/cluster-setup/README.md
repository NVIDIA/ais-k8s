## Cluster Setup Values

The values specified for each environment here are for general cluster operations such as node labeling or PV creation. 
They can be used for chart templating or parsed for scripts, but are not used for deployment or management of a specific resource.

Charts or scripts that use AIS deployment values are expected to receive those values separately. 

The only currently supported value is a list of cluster `nodes`. 