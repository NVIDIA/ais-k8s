#!/bin/bash

#
# Remove ais related data from all mountpath
# WARNING: will also cleanup data
#

set -e

mpaths=${MPATHS}

for m in ${mpaths}; do
        rm -rf $m/@ais
        rm -rf $m/@gcp
        rm -rf $m/@aws
        rm -rf $m/.ais.*
done
