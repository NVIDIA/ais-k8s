#!/bin/bash

if [ "$TEST_TYPE" = "" ]; then
    make test
else
    make "test-$TEST_TYPE"
fi
