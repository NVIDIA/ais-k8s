#!/bin/bash

#
# Script used for removing metadata stored on each mountpath.
# It should be executed on each storage target that needs a cleanup.
#

mpaths=${MPATHS}

for path in ${mpaths}; do
    rm -rf "${path}"/.ais.vmd
    rm -rf "${path}"/.ais.markers
done
