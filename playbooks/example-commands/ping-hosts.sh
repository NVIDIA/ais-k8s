#!/bin/bash -ue
ansible -v -i hosts.ini -m ping all
