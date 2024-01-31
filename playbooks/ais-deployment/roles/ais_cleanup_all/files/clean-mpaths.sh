#!/bin/bash

#
# Remove ais related data from all mountpath
# WARNING: will also cleanup data
#

mpaths=${MPATHS:-"/ais/sda /ais/sdb /ais/sdc /ais/sdd /ais/sde /ais/sdf /ais/sdg /ais/sdh /ais/sdi /ais/sdj"} # Adjust mpaths if needed.

for m in ${mpaths}; do
        rm -rf $m/@ais
        rm -rf $m/@gcp
        rm -rf $m/@aws
        rm -rf $m/.ais.*
done
