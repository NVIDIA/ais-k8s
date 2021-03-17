#!/bin/bash -ue
ansible -v -i hosts.ini --list-hosts all
