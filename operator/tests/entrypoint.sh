#!/bin/bash

if [ "$TEST_TYPE" = "" ]; then
    make test
else
    make "test-e2e-$TEST_TYPE"
fi
