#!/usr/bin/env bash

sudo rm -rf db
sudo rm -rf exports

mkdir db
mkdir exports 
sudo chown -R 1000:1000 $(pwd)/db
sudo chown -R 1000:1000 $(pwd)/exports