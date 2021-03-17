#!/bin/bash -ue
ansible-playbook -v -i hosts.ini ais_host_config_common.yml --check
