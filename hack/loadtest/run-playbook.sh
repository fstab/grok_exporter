#!/bin/bash

if [[ $# != 1 ]] ; then
    echo "Usage: ./run-playbook.sh <ip>"
    exit 1
fi

set -ex

ansible-playbook -i $1, -u $(whoami) playbook.yml
